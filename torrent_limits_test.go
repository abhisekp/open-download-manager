package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	rain "github.com/cenkalti/rain/v2/torrent"
)

func TestTorrentLimitPersistenceAndOwnerBoundaries(t *testing.T) {
	root := ownedTestDir(t)
	native, err := newNativeTorrents(root, filepath.Join(root, "files"), "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer native.Close()
	app := &App{engine: &Engine{Native: native}, token: "torrent-limit-owner", host: "127.0.0.1:9944"}
	for _, tc := range []struct {
		body, token, origin string
		code                int
	}{
		{`{"downloadKiB":64,"uploadKiB":16}`, "", "", 401},
		{`{"downloadKiB":64,"uploadKiB":16}`, "browser-token", "", 401},
		{`{"downloadKiB":64,"uploadKiB":16}`, app.token, "https://other.invalid", 403},
		{`{"downloadKiB":64}`, app.token, "", 400},
		{`{"downloadKiB":-1,"uploadKiB":16}`, app.token, "", 400},
		{`{"downloadKiB":1048577,"uploadKiB":0}`, app.token, "", 400},
		{`{"downloadKiB":0,"uploadKiB":0,"hidden":true}`, app.token, "", 400},
		{`{"downloadKiB":0,"uploadKiB":0} {}`, app.token, "", 400},
		{`{"downloadKiB":64,"uploadKiB":16}`, app.token, "http://" + app.host, 200},
	} {
		req := httptest.NewRequest("POST", "http://"+app.host+"/api/torrent-limits", strings.NewReader(tc.body))
		req.Header.Set("Authorization", "Bearer "+tc.token)
		req.Header.Set("Origin", tc.origin)
		out := httptest.NewRecorder()
		app.handler().ServeHTTP(out, req)
		if out.Code != tc.code {
			t.Fatalf("limits status %d want %d", out.Code, tc.code)
		}
	}
	file := native.limitFile
	saved, err := loadTorrentLimits(file)
	if err != nil || saved != (TorrentLimits{DownloadKiB: 64, UploadKiB: 16}) {
		t.Fatal("torrent limit setting not durable", err)
	}
	native.mu.Lock()
	native.limitFile = filepath.Join(ownedTestDir(t), "absent", "limits.json")
	native.mu.Unlock()
	if err := native.setLimits(TorrentLimits{}); err == nil {
		t.Fatal("failed limit write accepted")
	}
	actual, _ := native.getLimits()
	if actual != saved {
		t.Fatal("failed write changed active limits")
	}
	native.mu.Lock()
	native.limitFile = file
	native.mu.Unlock()
	if err := native.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := newNativeTorrents(root, filepath.Join(root, "files"), "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	actual, err = reopened.getLimits()
	if err != nil || actual != saved {
		t.Fatal("new engine did not restore caps", err)
	}
}

func TestNativeTorrentPayloadCapsAndLiveChange(t *testing.T) {
	for _, mode := range []string{"download", "upload", "live-change"} {
		t.Run(mode, func(t *testing.T) {
			payload := bytes.Repeat([]byte("original capped peer fixture\n"), 10000)
			metadata, hash := fixtureTorrent(t, "capped.bin", payload)
			seedRoot := ownedTestDir(t)
			seed, err := newNativeTorrents(seedRoot, filepath.Join(seedRoot, "files"), "127.0.0.1")
			if err != nil {
				t.Fatal(err)
			}
			defer seed.Close()
			seeder, err := seed.session.AddTorrent(bytes.NewReader(metadata), &rain.AddTorrentOptions{ID: "2222222222222222", Stopped: true})
			if err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(seeder.Dir(), "capped.bin")
			if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, payload, 0600); err != nil {
				t.Fatal(err)
			}
			if err := seeder.Start(); err != nil {
				t.Fatal(err)
			}
			waitNative(t, seeder)
			targetRoot := ownedTestDir(t)
			target, err := newNativeTorrents(targetRoot, filepath.Join(targetRoot, "files"), "127.0.0.1")
			if err != nil {
				t.Fatal(err)
			}
			defer target.Close()
			switch mode {
			case "upload":
				err = seed.setLimits(TorrentLimits{UploadKiB: 64})
			case "download":
				err = target.setLimits(TorrentLimits{DownloadKiB: 64})
			case "live-change":
				err = target.setLimits(TorrentLimits{DownloadKiB: 1})
			}
			if err != nil {
				t.Fatal(err)
			}
			start := time.Now()
			result, err := target.addURI("magnet:?xt=urn:btih:" + hash + "&x.pe=" + url.QueryEscape("127.0.0.1:"+strconv.Itoa(seeder.Stats().Port)))
			if err != nil {
				t.Fatal(err)
			}
			var gid string
			if json.Unmarshal(result, &gid) != nil {
				t.Fatal("invalid torrent ID")
			}
			transfer := target.session.GetTorrent(gid)
			if mode == "live-change" {
				deadline := time.Now().Add(10 * time.Second)
				for transfer.Stats().Pieces.Have < 1 {
					if time.Now().After(deadline) {
						t.Fatal("capped peer made no initial progress")
					}
					time.Sleep(10 * time.Millisecond)
				}
				start = time.Now()
				if err := target.setLimits(TorrentLimits{}); err != nil {
					t.Fatal(err)
				}
			}
			waitNative(t, transfer)
			elapsed := time.Since(start)
			if mode != "live-change" && elapsed < 2500*time.Millisecond {
				t.Fatal("native peer bypassed payload cap", elapsed)
			}
			if mode == "live-change" && elapsed > 5*time.Second {
				t.Fatal("existing peer retained obsolete slow reservation", elapsed)
			}
			data, err := os.ReadFile(filepath.Join(transfer.Dir(), "capped.bin"))
			if err != nil || !bytes.Equal(data, payload) {
				t.Fatal("capped transfer changed file bytes", err)
			}
		})
	}
}

func TestTorrentLimitCorruptionRetainsOriginal(t *testing.T) {
	for _, data := range []string{`{"version":2,"limits":{"downloadKiB":0,"uploadKiB":0}}`, `{"version":1,"limits":{"downloadKiB":0}}`, `{"version":1,"limits":{"downloadKiB":0,"uploadKiB":-1}}`, strings.Repeat("x", 1025)} {
		root := ownedTestDir(t)
		path := filepath.Join(root, "torrent-limits.json")
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := newNativeTorrents(root, filepath.Join(root, "files"), "127.0.0.1"); err == nil {
			t.Fatal("corrupt limits accepted")
		}
		actual, err := os.ReadFile(path)
		if err != nil || string(actual) != data {
			t.Fatal("original damaged limits changed", err)
		}
	}
}
