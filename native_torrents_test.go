package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	rain "github.com/cenkalti/rain/v2/torrent"
	"github.com/zeebo/bencode"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func fixtureTorrent(t *testing.T, name string, payload []byte) ([]byte, string) {
	t.Helper()
	var pieces []byte
	for start := 0; start < len(payload); start += 16384 {
		end := start + 16384
		if end > len(payload) {
			end = len(payload)
		}
		sum := sha1.Sum(payload[start:end])
		pieces = append(pieces, sum[:]...)
	}
	info := map[string]any{"name": name, "length": len(payload), "piece length": 16384, "pieces": string(pieces)}
	encoded, e := bencode.EncodeBytes(info)
	if e != nil {
		t.Fatal(e)
	}
	hash := sha1.Sum(encoded)
	metadata, e := bencode.EncodeBytes(map[string]any{"info": info})
	if e != nil {
		t.Fatal(e)
	}
	return metadata, hex.EncodeToString(hash[:])
}
func waitNative(t *testing.T, tor *rain.Torrent) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		s := normalizedTorrentStats(tor.Stats())
		if s.Error != nil {
			t.Fatal(s.Error)
		}
		if s.Bytes.Total > 0 && s.Bytes.Completed == s.Bytes.Total {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("local torrent did not finish: %+v peers=%+v", tor.Stats(), tor.Peers())
}
func TestNativeTorrentLocalPeerAndPersistence(t *testing.T) {
	payload := bytes.Repeat([]byte("ODM native torrent fixture\n"), 6000)
	metadata, hash := fixtureTorrent(t, "native-fixture.bin", payload)
	seedRoot := t.TempDir()
	seed, e := newNativeTorrents(seedRoot, filepath.Join(seedRoot, "files"), "127.0.0.1")
	if e != nil {
		t.Fatal(e)
	}
	defer seed.Close()
	seeder, e := seed.session.AddTorrent(bytes.NewReader(metadata), &rain.AddTorrentOptions{ID: "1111111111111111", Stopped: true})
	if e != nil {
		t.Fatal(e)
	}
	seedFile := filepath.Join(seeder.Dir(), "native-fixture.bin")
	os.MkdirAll(filepath.Dir(seedFile), 0700)
	if e = os.WriteFile(seedFile, payload, 0600); e != nil {
		t.Fatal(e)
	}
	if e = seeder.Start(); e != nil {
		t.Fatal(e)
	}
	waitNative(t, seeder)
	root := t.TempDir()
	downloads := filepath.Join(root, "files")
	native, e := newNativeTorrents(root, downloads, "127.0.0.1")
	if e != nil {
		t.Fatal(e)
	}
	closed := false
	defer func() {
		if !closed {
			native.Close()
		}
	}()
	raw, e := native.addURI("magnet:?xt=urn:btih:" + hash + "&dn=native-fixture.bin&x.pe=" + url.QueryEscape("127.0.0.1:"+strconv.Itoa(seeder.Port())))
	if e != nil {
		t.Fatal(e)
	}
	var gid string
	json.Unmarshal(raw, &gid)
	download := native.session.GetTorrent(gid)
	waitNative(t, download)
	row := native.row(download)
	var status struct {
		Native bool
		Status string
		Files  []struct{ Path string }
	}
	json.Unmarshal(row, &status)
	if !status.Native || status.Status != "complete" || len(status.Files) != 1 {
		t.Fatal(string(row))
	}
	got, e := os.ReadFile(status.Files[0].Path)
	if e != nil || !bytes.Equal(got, payload) {
		t.Fatal("native peer transfer integrity failed", e)
	}
	if e = native.Close(); e != nil {
		t.Fatal(e)
	}
	closed = true
	restored, e := newNativeTorrents(root, downloads, "127.0.0.1")
	if e != nil {
		t.Fatal(e)
	}
	defer restored.Close()
	if restored.session.GetTorrent(gid) == nil {
		t.Fatal("native torrent was not restored")
	}
}
func TestNativeTorrentRejectsUnsafeMetadata(t *testing.T) {
	root := t.TempDir()
	n, e := newNativeTorrents(root, filepath.Join(root, "files"), "127.0.0.1")
	if e != nil {
		t.Fatal(e)
	}
	defer n.Close()
	if _, e = n.addFile(strings.NewReader("not a torrent")); e == nil {
		t.Fatal("accepted invalid metadata")
	}
	metadata, _ := fixtureTorrent(t, "../../escape", []byte("test"))
	if _, e = n.addFile(bytes.NewReader(metadata)); e == nil {
		t.Fatal("accepted unsafe torrent path")
	}
}
