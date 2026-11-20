package torrent

import (
	"bytes"
	"crypto/sha1"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cenkalti/rain/v2/internal/resumer/boltdbresumer"
	"github.com/zeebo/bencode"
	"go.etcd.io/bbolt"
)

func TestExpiredSeedingDoesNotResumeOrAnnounce(t *testing.T) {
	var requests atomic.Int32
	tracker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte("d8:intervali60e5:peers0:e"))
	}))
	defer tracker.Close()
	root := t.TempDir()
	cfg := DefaultConfig
	cfg.Database = filepath.Join(root, "torrent.db")
	cfg.DataDir = filepath.Join(root, "files")
	cfg.RPCEnabled = false
	cfg.DHTEnabled = false
	cfg.BlocklistURL = ""
	cfg.ResumeOnStartup = true
	cfg.TrackerStopTimeout = 100 * time.Millisecond
	session, err := NewSession(cfg)
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	defer func() {
		if !closed {
			session.Close()
		}
	}()
	payload := []byte("original fixture")
	sum := sha1.Sum(payload)
	metadata, err := bencode.EncodeBytes(map[string]any{"announce": tracker.URL, "info": map[string]any{"name": "original.bin", "length": len(payload), "piece length": 16384, "pieces": string(sum[:])}})
	if err != nil {
		t.Fatal(err)
	}
	tor, err := session.AddTorrent(bytes.NewReader(metadata), &AddTorrentOptions{ID: "4444444444444444", Stopped: true, StopAfterDownload: true})
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(tor.Dir(), 0700)
	if err = os.WriteFile(filepath.Join(tor.Dir(), "original.bin"), payload, 0600); err != nil {
		t.Fatal(err)
	}
	if err = tor.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !tor.SeedingState().Ready {
		if time.Now().After(deadline) {
			t.Fatal("fixture did not verify and stop")
		}
		time.Sleep(time.Millisecond)
	}
	if err = tor.SetSeedingDeadline(time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err = session.Close(); err != nil {
		t.Fatal(err)
	}
	closed = true
	// Model a process crash with Started=true and a deadline that elapsed while
	// the application was absent, without relying on wall-clock sleeps.
	db, err := bbolt.Open(cfg.Database, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(torrentsBucket).Bucket([]byte(tor.ID()))
		if err := b.Put(seedDeadlineKey, []byte(strconv.FormatInt(time.Now().Add(-time.Minute).Unix(), 10))); err != nil {
			return err
		}
		return b.Put(boltdbresumer.Keys.Started, []byte("true"))
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	before := requests.Load()
	restored, err := NewSession(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	resumed := restored.GetTorrent(tor.ID())
	if resumed == nil {
		t.Fatal("expired torrent disappeared")
	}
	if resumed.Stats().Status != Stopped || resumed.SeedingState().Active {
		t.Fatal("expired seed resumed")
	}
	// Even an explicit ordinary resume must retain stop-on-complete semantics.
	if err = resumed.Start(); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(5 * time.Second)
	for resumed.Stats().Status != Stopped {
		if time.Now().After(deadline) {
			t.Fatal("ordinary resume bypassed expired policy")
		}
		time.Sleep(time.Millisecond)
	}
	if requests.Load() != before {
		t.Fatal("expired policy contacted tracker")
	}
	previous := resumed.SeedingState()
	if err = restored.db.Close(); err != nil {
		t.Fatal(err)
	}
	if err = resumed.SetSeedingDeadline(time.Now().Add(time.Minute)); err == nil {
		t.Fatal("failed storage accepted sharing consent")
	}
	if resumed.SeedingState() != previous {
		t.Fatal("failed seed save changed active policy")
	}
	if err = resumed.SetSeedingDeadline(time.Time{}); err == nil {
		t.Fatal("failed stop persistence not reported")
	}
	if state := resumed.SeedingState(); state.Active || state.Until != 0 {
		t.Fatal("failed stop persistence retained runtime sharing")
	}
}
