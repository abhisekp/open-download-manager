package main

import (
	"bytes"
	"encoding/json"
	"errors"
	rainlog "github.com/cenkalti/log"
	rain "github.com/cenkalti/rain/v2/torrent"
	"github.com/zeebo/bencode"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Native BitTorrent runs in-process. It does not send torrent operations to aria2.
type NativeTorrents struct{ session *rain.Session }

var nativeLogInitialization sync.Once

type discardTorrentLog struct{}

func (discardTorrentLog) SetFormatter(rainlog.Formatter) {}
func (discardTorrentLog) SetLevel(rainlog.Level)         {}
func (discardTorrentLog) Handle(*rainlog.Record)         {}
func (discardTorrentLog) Close() error                   { return nil }
func newNativeTorrents(data, downloads, host string) (*NativeTorrents, error) {
	cfg := rain.DefaultConfig
	cfg.Database = filepath.Join(data, "torrents.db")
	cfg.DataDir = downloads
	cfg.Host = host
	cfg.RPCEnabled = false
	cfg.DHTEnabled = false
	cfg.PEXEnabled = false
	cfg.BlocklistURL = ""
	cfg.OnCompleteCmd = nil
	cfg.ResumeOnStartup = true
	cfg.ResumeWriteInterval = 5 * time.Second
	cfg.TrackerStopTimeout = time.Second
	cfg.TrackerHTTPVerifyTLS = true
	cfg.WebseedVerifyTLS = true
	cfg.MaxTorrentSize = 4 << 20
	cfg.MaxMetadataSize = 4 << 20
	cfg.MaxPeerDial = 32
	cfg.MaxPeerAccept = 32
	cfg.FilePermissions = 0700
	var session *rain.Session
	var e error
	// Rain's handler is process-global. Replacing it for a second session races
	// with logger creation by the first session's peer goroutines.
	nativeLogInitialization.Do(func() {
		cfg.CustomLogHandler = discardTorrentLog{}
		session, e = rain.NewSession(cfg)
	})
	if session == nil && e == nil {
		cfg.CustomLogHandler = nil
		session, e = rain.NewSession(cfg)
	}
	if e != nil {
		return nil, e
	}
	return &NativeTorrents{session}, nil
}
func (n *NativeTorrents) Close() error { return n.session.Close() }
func torrentLink(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && (u.Scheme == "magnet" || ((u.Scheme == "https" || u.Scheme == "http") && strings.HasSuffix(strings.ToLower(u.Path), ".torrent")))
}
func (n *NativeTorrents) addURI(uri string) (json.RawMessage, error) {
	if e := validateURI(uri); e != nil {
		return nil, e
	}
	if len(n.session.ListTorrents()) >= 200 {
		return nil, errors.New("native torrent limit reached (200)")
	}
	if !strings.HasPrefix(uri, "magnet:") {
		client := &http.Client{Timeout: 15 * time.Second}
		res, err := client.Get(uri)
		if err != nil {
			return nil, errors.New("torrent metadata could not be downloaded")
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return nil, errors.New("torrent metadata server returned an error")
		}
		return n.addFile(res.Body)
	}
	gid := token()[:16]
	t, e := n.session.AddURI(uri, &rain.AddTorrentOptions{ID: gid, StopAfterDownload: true})
	if e != nil {
		return nil, errors.New("native torrent engine rejected this link")
	}
	return json.Marshal(t.ID())
}
func (n *NativeTorrents) addFile(r io.Reader) (json.RawMessage, error) {
	if len(n.session.ListTorrents()) >= 200 {
		return nil, errors.New("native torrent limit reached (200)")
	}
	data, err := io.ReadAll(io.LimitReader(r, (4<<20)+1))
	if err != nil || len(data) > 4<<20 {
		return nil, errors.New("torrent metadata exceeds 4 MiB")
	}
	var meta struct {
		Info struct {
			Name  string `bencode:"name"`
			Files []struct {
				Path []string `bencode:"path"`
			} `bencode:"files"`
		} `bencode:"info"`
	}
	if bencode.DecodeBytes(data, &meta) != nil || !safeTorrentPart(meta.Info.Name) {
		return nil, errors.New("invalid torrent file name")
	}
	for _, f := range meta.Info.Files {
		if len(f.Path) == 0 {
			return nil, errors.New("invalid torrent path")
		}
		for _, part := range f.Path {
			if !safeTorrentPart(part) {
				return nil, errors.New("unsafe torrent path")
			}
		}
	}
	gid := token()[:16]
	t, e := n.session.AddTorrent(bytes.NewReader(data), &rain.AddTorrentOptions{ID: gid, StopAfterDownload: true})
	if e != nil {
		return nil, errors.New("invalid or unsupported torrent metadata")
	}
	return json.Marshal(t.ID())
}
func safeTorrentPart(part string) bool {
	base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
	reserved := base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9')
	return !reserved && part != "" && part != "." && part != ".." && !strings.ContainsAny(part, "/\\:\x00") && !strings.HasSuffix(part, ".") && !strings.HasSuffix(part, " ")
}
func nativeStatus(s rain.Stats) string {
	s = normalizedTorrentStats(s)
	if s.Error != nil {
		return "error"
	}
	if s.Bytes.Total > 0 && s.Bytes.Completed == s.Bytes.Total {
		return "complete"
	}
	if s.Status == rain.Stopped || s.Status == rain.Stopping {
		return "paused"
	}
	return "active"
}

// Rain clears active piece structures on stop, so byte totals can read zero
// despite a complete persisted bitfield. All verified pieces establish completion.
func normalizedTorrentStats(s rain.Stats) rain.Stats {
	if s.Pieces.Total > 0 && s.Pieces.Have == s.Pieces.Total {
		s.Bytes.Completed = s.Bytes.Total
		s.Bytes.Incomplete = 0
	}
	return s
}
func (n *NativeTorrents) row(t *rain.Torrent) json.RawMessage {
	s := normalizedTorrentStats(t.Stats())
	files := []map[string]any{}
	if fs, e := t.Files(); e == nil {
		for _, f := range fs {
			files = append(files, map[string]any{"path": filepath.Join(t.Dir(), f.Path()), "length": strconv.FormatInt(f.Length(), 10), "uris": []any{}})
		}
	}
	message := ""
	if s.Error != nil {
		message = "Native torrent transfer failed; inspect source and storage availability."
	}
	raw, _ := json.Marshal(map[string]any{"gid": t.ID(), "status": nativeStatus(s), "totalLength": strconv.FormatInt(s.Bytes.Total, 10), "completedLength": strconv.FormatInt(s.Bytes.Completed, 10), "downloadSpeed": strconv.Itoa(s.Speed.Download), "uploadSpeed": strconv.Itoa(s.Speed.Upload), "connections": strconv.Itoa(s.Peers.Total), "files": files, "bittorrent": map[string]any{"info": map[string]string{"name": s.Name}}, "infoHash": s.InfoHash.String(), "native": true, "torrentState": s.Status.String(), "private": s.Private, "errorMessage": message})
	return raw
}
func (n *NativeTorrents) rows(method string) []json.RawMessage {
	out := []json.RawMessage{}
	for _, t := range n.session.ListTorrents() {
		s := nativeStatus(t.Stats())
		if (method == "tellActive" && s == "active") || (method == "tellWaiting" && s == "paused") || (method == "tellStopped" && (s == "complete" || s == "error")) {
			out = append(out, n.row(t))
		}
	}
	return out
}
func (n *NativeTorrents) control(method, gid string) (bool, json.RawMessage, error) {
	t := n.session.GetTorrent(gid)
	if t == nil {
		return false, nil, nil
	}
	var err error
	switch method {
	case "tellStatus":
		return true, n.row(t), nil
	case "pause":
		err = t.Stop()
	case "unpause":
		err = t.Start()
	case "remove", "removeDownloadResult":
		err = n.session.RemoveTorrent(gid, true)
	default:
		return false, nil, nil
	}
	raw, _ := json.Marshal(gid)
	return true, raw, err
}
