package torrent

import (
	"bytes"
	"crypto/sha1"
	"testing"

	"github.com/zeebo/bencode"
)

func TestReplacementWriteFailureKeepsPreviousTrackers(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig
	cfg.Database = root + "/torrent.db"
	cfg.DataDir = root + "/files"
	cfg.RPCEnabled = false
	cfg.DHTEnabled = false
	cfg.BlocklistURL = ""
	session, err := NewSession(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	sum := sha1.Sum([]byte("original fixture"))
	metadata, err := bencode.EncodeBytes(map[string]any{"announce": "http://127.0.0.1:9/original", "info": map[string]any{"name": "original.bin", "length": 16, "piece length": 16384, "pieces": string(sum[:])}})
	if err != nil {
		t.Fatal(err)
	}
	tor, err := session.AddTorrent(bytes.NewReader(metadata), &AddTorrentOptions{ID: "1111111111111111", Stopped: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = session.db.Close(); err != nil {
		t.Fatal(err)
	}
	if err = tor.ReplaceTrackers([]string{"http://127.0.0.1:9/original"}, []string{"http://127.0.0.1:9/replacement"}); err == nil {
		t.Fatal("closed storage accepted replacement")
	}
	actual := query(tor.torrent, tor.torrent.getTieredTrackers)
	if len(actual) != 1 || len(actual[0]) != 1 || actual[0][0] != "http://127.0.0.1:9/original" {
		t.Fatal("failed transaction changed active trackers")
	}
}
