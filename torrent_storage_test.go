package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	rain "github.com/cenkalti/rain/v2/torrent"
)

func TestNativeTorrentRejectsHardLinkAndPreservesOriginal(t *testing.T) {
	root := ownedTestDir(t)
	n, err := newNativeTorrents(root, filepath.Join(root, "files"), "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	payload := []byte("original private file fixture")
	original := filepath.Join(root, "private-original.bin")
	if err = os.WriteFile(original, payload, 0600); err != nil {
		t.Fatal(err)
	}
	metadata, _ := fixtureTorrent(t, "payload.bin", payload)
	tor, err := n.session.AddTorrent(bytes.NewReader(metadata), &rain.AddTorrentOptions{ID: "5555555555555555", Stopped: true, StopAfterDownload: true})
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(tor.Dir(), 0700)
	if err = os.Link(original, filepath.Join(tor.Dir(), "payload.bin")); err != nil {
		t.Skip("fixture hard links unavailable:", err)
	}
	if err = tor.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for tor.Stats().Error == nil {
		if time.Now().After(deadline) {
			t.Fatal("linked destination was not rejected")
		}
		time.Sleep(time.Millisecond)
	}
	var row struct{ Status, ErrorMessage string }
	raw := n.row(tor)
	if json.Unmarshal(raw, &row) != nil || row.Status != "error" || !strings.Contains(row.ErrorMessage, "Original files were kept") {
		t.Fatal("storage recovery message missing")
	}
	if strings.Contains(string(raw), "private-original.bin") {
		t.Fatal("private hard-link destination disclosed")
	}
	actual, err := os.ReadFile(original)
	if err != nil || !bytes.Equal(actual, payload) {
		t.Fatal("original hard-link bytes changed", err)
	}
}
