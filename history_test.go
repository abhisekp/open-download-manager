package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHistoryPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	h, e := openHistory(path)
	if e != nil {
		t.Fatal(e)
	}
	rows := []json.RawMessage{json.RawMessage(`{"gid":"a","status":"complete","files":[{"path":"file"}]}`), json.RawMessage(`{"gid":"b","status":"active"}`)}
	if _, e = h.merge(rows); e != nil {
		t.Fatal(e)
	}
	h, e = openHistory(path)
	if e != nil {
		t.Fatal(e)
	}
	got, e := h.merge(nil)
	if e != nil || len(got) != 1 || !strings.Contains(string(got[0]), `"archived":true`) {
		t.Fatal(string(got[0]), e)
	}
	if e = h.forget("a"); e != nil {
		t.Fatal(e)
	}
	h, _ = openHistory(path)
	got, _ = h.merge(nil)
	if len(got) != 0 {
		t.Fatal("forget did not persist")
	}
}
func TestCorruptHistoryPreserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	os.WriteFile(path, []byte("bad"), 0600)
	if _, e := openHistory(path); e == nil {
		t.Fatal("accepted corrupt history")
	}
	b, _ := os.ReadFile(path)
	if string(b) != "bad" {
		t.Fatal("overwrote history")
	}
}
func TestChecksumBoundary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file")
	os.WriteFile(path, []byte("verified"), 0600)
	sum := sha256.Sum256([]byte("verified"))
	digest := hex.EncodeToString(sum[:])
	got, match, e := verifyFile(context.Background(), dir, path, digest)
	if e != nil || !match || got != digest {
		t.Fatal(got, match, e)
	}
	_, match, e = verifyFile(context.Background(), dir, path, strings.Repeat("0", 64))
	if e != nil || match {
		t.Fatal("mismatch not reported")
	}
	if _, _, e = verifyFile(context.Background(), dir, path, "bad"); e == nil {
		t.Fatal("accepted malformed digest")
	}
	outside := filepath.Join(t.TempDir(), "private")
	os.WriteFile(outside, []byte("secret"), 0600)
	if _, _, e = verifyFile(context.Background(), dir, outside, digest); e == nil {
		t.Fatal("opened outside root")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, e = verifyFile(ctx, dir, path, digest); e == nil {
		t.Fatal("ignored cancellation")
	}
}
