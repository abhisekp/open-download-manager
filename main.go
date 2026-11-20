package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/abhisekp/open-download-manager/internal/addons"
	"github.com/abhisekp/open-download-manager/internal/processutil"
	"github.com/abhisekp/open-download-manager/internal/transfers"
)

//go:embed ui/dist/client
var assets embed.FS

//go:embed VERSION
var versionFile string

var version = strings.TrimSpace(versionFile)

func token() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func main() {
	data := flag.String("data-dir", "", "application data directory")
	downloads := flag.String("downloads", "", "download directory")
	port := flag.Int("port", 0, "loopback UI port (0 chooses a free port)")
	enginePath := flag.String("engine", "aria2c", "path to aria2c")
	noBrowser := flag.Bool("no-browser", false, "do not open a browser")
	showVersion := flag.Bool("version", false, "print version")
	addonWorker := flag.Bool("addon-worker", false, "run the isolated add-on worker")
	flag.Parse()
	if *addonWorker {
		if addons.Worker(os.Stdin, os.Stdout, os.Stderr) != nil {
			os.Exit(1)
		}
		return
	}
	if *showVersion {
		fmt.Println(version)
		return
	}
	home, _ := os.UserHomeDir()
	if *data == "" {
		base, e := os.UserConfigDir()
		if e != nil {
			log.Fatal(e)
		}
		*data = filepath.Join(base, "OpenDownloadManager")
	}
	if *downloads == "" {
		*downloads = filepath.Join(home, "Downloads", "OpenDownloadManager")
	}
	for _, p := range []string{*data, *downloads} {
		if strings.ContainsAny(p, "\r\n") {
			log.Fatal("invalid directory")
		}
		if e := os.MkdirAll(p, 0700); e != nil {
			log.Fatal(e)
		}
	}
	lock, err := acquireDataLock(*data)
	if err != nil {
		log.Fatal(err)
	}
	defer lock.Close()
	rpcListener, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		log.Fatal(e)
	}
	rpcPort := rpcListener.Addr().(*net.TCPAddr).Port
	rpcListener.Close()
	secret := token()
	session := filepath.Join(*data, "session.txt")
	if _, e = os.Stat(session); os.IsNotExist(e) {
		if e = os.WriteFile(session, nil, 0600); e != nil {
			log.Fatal(e)
		}
	}
	cfg := filepath.Join(*data, "engine.conf")
	config := fmt.Sprintf("enable-rpc=true\nrpc-listen-all=false\nrpc-listen-port=%d\nrpc-secret=%s\ndir=%s\ninput-file=%s\nsave-session=%s\nsave-session-interval=10\ncontinue=true\nmax-concurrent-downloads=3\nsplit=4\nmax-connection-per-server=4\nseed-time=0\nbt-save-metadata=true\nenable-dht=false\nenable-dht6=false\nenable-peer-exchange=false\n", rpcPort, secret, filepath.ToSlash(*downloads), filepath.ToSlash(session), filepath.ToSlash(session))
	if e = os.WriteFile(cfg, []byte(config), 0600); e != nil {
		log.Fatal(e)
	}
	defer os.Remove(cfg)
	native, err := newNativeTorrents(*data, *downloads, "0.0.0.0")
	if err != nil {
		log.Print("Native torrent engine unavailable; its database is retained for recovery")
	} else {
		defer native.Close()
	}
	engine := newEngine("", secret)
	engine.Native = native
	direct, err := transfers.Open(transfers.Config{DataDir: *data, Downloads: *downloads})
	if err != nil {
		log.Print("Native HTTP registry could not be opened; its data is retained for recovery")
	} else {
		engine.HTTP = direct
		engine.HTTPDirectory = *downloads
		defer direct.Close()
	}
	storedLimit := uint64(0)
	if direct != nil {
		storedLimit = direct.Limit()
	}
	cmd := exec.Command(*enginePath, "--conf-path="+cfg, "--console-log-level=error", "--summary-interval=0", "--max-overall-download-limit="+strconv.FormatUint(storedLimit, 10))
	processutil.Background(cmd)
	if err = cmd.Start(); err != nil {
		log.Print("Optional aria2 engine is unavailable; FTP/SFTP require it. Settings reports built-in engine availability.")
	} else {
		defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
		engine.URL = fmt.Sprintf("http://127.0.0.1:%d/jsonrpc", rpcPort)
		ready := false
		for i := 0; i < 40; i++ {
			if _, e = engine.Call(context.Background(), "getVersion"); e == nil {
				ready = true
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if !ready {
			log.Print("Optional aria2 engine failed to start; built-in engines remain available")
			engine.URL = ""
		}
	}
	listener, e := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(*port))
	if e != nil {
		log.Print(e)
		return
	}
	auth := token()
	addr := "http://" + listener.Addr().String()
	history, err := openHistory(filepath.Join(*data, "history.json"))
	if err != nil {
		log.Print(err)
		return
	}
	app := &App{engine: engine, token: auth, host: listener.Addr().String(), downloads: *downloads, history: history, verifyGate: make(chan struct{}, 1)}
	app.addons, err = addons.Open(filepath.Join(*data, "addons"))
	if err != nil {
		log.Print("Add-on registry unavailable; ordinary downloads remain available")
	}
	app.bridge = &Bridge{}
	srv := &http.Server{Handler: app.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	app.shutdown = stop
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if raw, e := engine.Call(ctx, "tellStopped", 0, 1000); e == nil {
					var rows []json.RawMessage
					_ = json.Unmarshal(raw, &rows)
					if _, e = history.merge(rows); e != nil {
						log.Print("Unable to save completed history")
					}
				}
			}
		}
	}()
	defer stop()
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if raw, e := engine.Call(c, "tellStopped", 0, 1000); e == nil {
			var rows []json.RawMessage
			_ = json.Unmarshal(raw, &rows)
			_, _ = history.merge(rows)
		}
		_, _ = engine.Call(c, "saveSession")
		_, _ = engine.Call(c, "shutdown")
		_ = srv.Shutdown(c)
	}()
	launch := addr + "/#" + auth
	fmt.Println("Open Download Manager:", launch)
	if !*noBrowser {
		openBrowser(launch)
	}
	if e = srv.Serve(listener); e != nil && !errors.Is(e, http.ErrServerClosed) {
		log.Print(e)
	}
}
func openBrowser(u string) {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	case "darwin":
		c = exec.Command("open", u)
	default:
		c = exec.Command("xdg-open", u)
	}
	if c.Start() == nil {
		go c.Wait()
	}
}

type App struct {
	engine                 *Engine
	token, host, downloads string
	shutdown               func()
	history                *History
	verifyGate             chan struct{}
	bridge                 *Bridge
	addons                 *addons.Store
}

func (a *App) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/torrent-trackers", a.torrentTrackersAPI)
	mux.HandleFunc("/api/http-queue", a.httpQueueAPI)
	mux.HandleFunc("/api/addons", a.addonsAPI)
	mux.HandleFunc("/api/addons/", a.addonsAPI)
	mux.HandleFunc("/api/", a.api)
	mux.HandleFunc("/bridge/", a.bridgeAPI)
	files, _ := fs.Sub(assets, "ui/dist/client")
	mux.Handle("/", http.FileServer(http.FS(files)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; font-src 'self'; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		if r.Host != a.host {
			http.Error(w, "invalid host", 403)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+a.host {
				http.Error(w, "invalid origin", 403)
				return
			}
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+a.token)) != 1 {
				http.Error(w, "unauthorized", 401)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}
func respond(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func (a *App) api(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		switch r.URL.Path {
		case "/api/status":
			var all []json.RawMessage
			for _, q := range []struct {
				name   string
				params []any
			}{{"tellActive", nil}, {"tellWaiting", []any{0, 1000}}, {"tellStopped", []any{0, 1000}}} {
				raw, e := a.engine.Call(r.Context(), q.name, q.params...)
				if e != nil {
					http.Error(w, "Cannot read transfer status", 502)
					return
				}
				var rows []json.RawMessage
				_ = json.Unmarshal(raw, &rows)
				all = append(all, rows...)
			}
			var historyErr error
			all, historyErr = a.history.merge(all)
			if historyErr != nil {
				http.Error(w, "Cannot persist download history", 500)
				return
			}
			stats, _ := a.engine.Call(r.Context(), "getGlobalStat")
			limit := uint64(0)
			if a.engine.HTTP != nil {
				limit = a.engine.HTTP.Limit()
			}
			respond(w, map[string]any{"downloads": all, "stats": stats, "directory": a.downloads, "version": version, "settings": map[string]any{"downloadLimit": strconv.FormatUint(limit, 10)}, "capabilities": map[string]bool{"http": a.engine.HTTP != nil || a.engine.ariaAvailable(), "torrents": a.engine.Native != nil, "ftp_sftp": a.engine.ariaAvailable()}})
			return
		case "/api/extensions":
			respond(w, []Manifest{trackerManifest})
			return
		}
	}
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	if r.URL.Path == "/api/torrent-file" {
		if a.engine.Native == nil {
			http.Error(w, "native torrents unavailable", 503)
			return
		}
		raw, err := a.engine.Native.addFile(http.MaxBytesReader(w, r.Body, 4<<20))
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		respond(w, map[string]any{"result": raw})
		return
	}
	var in struct {
		URL             string   `json:"url"`
		GID             string   `json:"gid"`
		Action          string   `json:"action"`
		Limit           string   `json:"limit"`
		Enabled         bool     `json:"enabled"`
		Checksum        string   `json:"checksum"`
		URLs            []string `json:"urls"`
		ExtensionOrigin string   `json:"extensionOrigin"`
	}
	if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&in); e != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	var out json.RawMessage
	var e error
	switch r.URL.Path {
	case "/api/pair":
		if a.bridge == nil {
			http.Error(w, "browser bridge unavailable", 503)
			return
		}
		secret, ok := a.bridge.pair(in.ExtensionOrigin)
		if !ok {
			http.Error(w, "paste the browser extension identity from its popup", 400)
			return
		}
		respond(w, map[string]string{"endpoint": "http://" + a.host, "origin": in.ExtensionOrigin, "token": secret})
		return
	case "/api/revoke-browsers":
		if a.bridge != nil {
			a.bridge.revoke()
		}
		respond(w, map[string]bool{"revoked": true})
		return
	case "/api/quit":
		respond(w, map[string]any{"message": "Stopping and saving transfers"})
		if a.shutdown != nil {
			go func() { time.Sleep(100 * time.Millisecond); a.shutdown() }()
		}
		return
	case "/api/verify":
		if a.verifyGate == nil {
			http.Error(w, "verifier unavailable", 503)
			return
		}
		select {
		case a.verifyGate <- struct{}{}:
			defer func() { <-a.verifyGate }()
		default:
			http.Error(w, "another verification is running", 429)
			return
		}
		result, err := a.verify(r.Context(), in.GID, in.Checksum)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		respond(w, result)
		return
	case "/api/batch":
		if len(in.URLs) == 0 || len(in.URLs) > 50 {
			http.Error(w, "provide between 1 and 50 links", 400)
			return
		}
		unique := []string{}
		seen := map[string]bool{}
		for _, u := range in.URLs {
			u = strings.TrimSpace(u)
			if err := validateURI(u); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			if !seen[u] {
				unique = append(unique, u)
				seen[u] = true
			}
		}
		results := []map[string]any{}
		for _, u := range unique {
			raw, err := a.engine.Call(r.Context(), "addUri", []string{u})
			if err != nil {
				results = append(results, map[string]any{"url": u, "error": "engine rejected link"})
			} else {
				results = append(results, map[string]any{"url": u, "gid": raw})
			}
		}
		_, _ = a.engine.Call(r.Context(), "saveSession")
		respond(w, map[string]any{"results": results})
		return
	case "/api/add":
		if e = validateURI(strings.TrimSpace(in.URL)); e == nil {
			out, e = a.engine.Call(r.Context(), "addUri", []string{strings.TrimSpace(in.URL)})
		}
	case "/api/control":
		if len(in.GID) != 16 {
			http.Error(w, "invalid transfer ID", 400)
			return
		}
		if _, err := hex.DecodeString(in.GID); err != nil {
			http.Error(w, "invalid transfer ID", 400)
			return
		}
		methods := map[string]string{"pause": "pause", "resume": "unpause", "remove": "remove", "forget": "removeDownloadResult"}
		method, ok := methods[in.Action]
		if !ok {
			http.Error(w, "invalid action", 400)
			return
		}
		if in.Action == "forget" && len(a.history.get(in.GID)) > 0 {
			_, _ = a.engine.Call(r.Context(), method, in.GID)
			e = a.history.forget(in.GID)
		} else {
			out, e = a.engine.Call(r.Context(), method, in.GID)
		}
	case "/api/limits":
		if _, err := strconv.ParseUint(in.Limit, 10, 64); err != nil {
			http.Error(w, "use bytes per second, or 0 for unlimited", 400)
			return
		}
		out, e = a.engine.Call(r.Context(), "changeGlobalOption", map[string]string{"max-overall-download-limit": in.Limit})
	case "/api/engine-check":
		if a.engine.URL == "" {
			http.Error(w, "Optional aria2 engine is not running; start ODM with aria2 installed to enable FTP/SFTP", 400)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		out, e = a.engine.Call(ctx, "getVersion")
	case "/api/trackers":
		if !in.Enabled {
			http.Error(w, "grant this extension network access first", 403)
			return
		}
		rows, err := fetchTrackers(r.Context(), trackerManifest)
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		respond(w, map[string]any{"trackers": rows})
		return
	default:
		http.NotFound(w, r)
		return
	}
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	_, _ = a.engine.Call(r.Context(), "saveSession")
	respond(w, map[string]any{"result": out})
}
