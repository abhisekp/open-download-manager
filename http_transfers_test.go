package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/abhisekp/open-download-manager/internal/transfers"
)

func TestNativeHTTPAdapterSurvivesOptionalEngineFailure(t *testing.T) {
	fixture := []byte("original fallback adapter fixture")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(fixture) }))
	defer server.Close()
	rpc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "invalid RPC", 500) }))
	address := rpc.URL
	rpc.Close()
	dir := t.TempDir()
	direct, err := transfers.Open(transfers.Config{DataDir: filepath.Join(dir, "data"), Downloads: filepath.Join(dir, "files")})
	if err != nil {
		t.Fatal(err)
	}
	defer direct.Close()
	engine := newEngine(address, "secret")
	engine.HTTP = direct
	engine.HTTPDirectory = filepath.Join(dir, "files")
	if _, err = engine.Call(context.Background(), "tellActive"); err != nil {
		t.Fatal("optional RPC failure hid local status", err)
	}
	if engine.ariaAvailable() {
		t.Fatal("failed optional engine still advertised as available")
	}
	raw, err := engine.Call(context.Background(), "addUri", []string{server.URL + "/file.bin"})
	if err != nil {
		t.Fatal(err)
	}
	var id string
	if json.Unmarshal(raw, &id) != nil {
		t.Fatal("invalid fallback identifier")
	}
	deadline := time.Now().Add(10 * time.Second)
	completed := false
	for time.Now().Before(deadline) {
		raw, err = engine.Call(context.Background(), "tellStatus", id)
		if err != nil {
			t.Fatal(err)
		}
		var row struct{ Status string }
		_ = json.Unmarshal(raw, &row)
		if row.Status == "complete" {
			completed = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !completed {
		t.Fatal("fallback download did not complete")
	}
	app := &App{engine: engine, downloads: engine.HTTPDirectory}
	digest := sha256.Sum256(fixture)
	result, err := app.verify(context.Background(), id, hex.EncodeToString(digest[:]))
	if err != nil || result["matches"] != true {
		t.Fatal("fallback checksum failed", result, err)
	}
	if _, err = engine.Call(context.Background(), "addUri", []string{"magnet:?xt=urn:btih:0000000000000000000000000000000000000000"}); err == nil {
		t.Fatal("missing native torrent engine was silently redirected")
	}
}
