package torrent

import (
	"bytes"
	"crypto/sha1"
	"math"
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

func TestSeedingRatioTargetArithmetic(t *testing.T) {
	for _, tc := range []struct {
		size    int64
		percent int
		want    int64
	}{
		{1, 1, 1}, {101, 1, 2}, {101, 100, 101}, {101, 150, 152}, {101, 10000, 10100}, {math.MaxInt64, 100, math.MaxInt64},
	} {
		got, err := seedingRatioTarget(tc.size, tc.percent)
		if err != nil || got != tc.want {
			t.Fatalf("size=%d percent=%d target=%d error=%v", tc.size, tc.percent, got, err)
		}
	}
	for _, tc := range []struct {
		size    int64
		percent int
	}{{0, 100}, {-1, 100}, {100, 0}, {100, 10001}, {math.MaxInt64, 101}, {math.MaxInt64, 10000}} {
		if _, err := seedingRatioTarget(tc.size, tc.percent); err == nil {
			t.Fatalf("accepted invalid or overflowing target: %+v", tc)
		}
	}
}

func TestSeedingRatioPersistenceAndReachedCrashState(t *testing.T) {
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
	cfg.Host = "127.0.0.1"
	cfg.RPCEnabled = false
	cfg.DHTEnabled = false
	cfg.BlocklistURL = ""
	cfg.ResumeOnStartup = true
	cfg.TrackerStopTimeout = 100 * time.Millisecond
	session, err := NewSession(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if session != nil {
			session.Close()
		}
	}()
	payload := []byte("original ratio fixture")
	sum := sha1.Sum(payload)
	metadata, err := bencode.EncodeBytes(map[string]any{"announce": tracker.URL, "info": map[string]any{"name": "ratio.bin", "length": len(payload), "piece length": 16384, "pieces": string(sum[:])}})
	if err != nil {
		t.Fatal(err)
	}
	tor, err := session.AddTorrent(bytes.NewReader(metadata), &AddTorrentOptions{ID: "5555555555555555", Stopped: true, StopAfterDownload: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(tor.Dir(), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(tor.Dir(), "ratio.bin"), payload, 0600); err != nil {
		t.Fatal(err)
	}
	if err = tor.Start(); err != nil {
		t.Fatal(err)
	}
	waitRatioPaused(t, tor)
	for _, ratio := range []int{-1, 10001} {
		if err = tor.SetSeedingPolicy(time.Now().Add(time.Minute), ratio); err == nil {
			t.Fatal("accepted invalid ratio")
		}
	}
	if err = tor.SetSeedingPolicy(time.Time{}, 100); err == nil {
		t.Fatal("accepted ratio without time bound")
	}
	if err = tor.SetSeedingPolicy(time.Now().Add(time.Minute), 150); err != nil {
		t.Fatal(err)
	}
	policy := tor.SeedingState()
	if policy.RatioPercent != 150 || policy.TargetBytes != int64((len(payload)*150+99)/100) {
		t.Fatal("wrong reviewed ratio", policy)
	}
	if err = session.Close(); err != nil {
		t.Fatal(err)
	}
	session = nil
	restored, err := NewSession(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if restored != nil {
			restored.Close()
		}
	}()
	continued := restored.GetTorrent(tor.ID())
	if continued == nil {
		t.Fatal("ratio torrent missing")
	}
	state := continued.SeedingState()
	if state.RatioPercent != 150 || state.TargetBytes != policy.TargetBytes || state.Until != policy.Until {
		t.Fatal("restart changed ratio consent", state)
	}
	if err = restored.Close(); err != nil {
		t.Fatal(err)
	}
	restored = nil
	// Model a crash with Started=true, a future deadline and an already reached
	// persisted upload target. No tracker contact or peer sharing may resume.
	db, err := bbolt.Open(cfg.Database, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(torrentsBucket).Bucket([]byte(tor.ID()))
		if err := b.Put(boltdbresumer.Keys.BytesUploaded, []byte(strconv.FormatInt(policy.TargetBytes, 10))); err != nil {
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
	reached, err := NewSession(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reached.Close()
	finished := reached.GetTorrent(tor.ID())
	if finished == nil {
		t.Fatal("reached torrent missing")
	}
	if finished.Stats().Status != Stopped {
		t.Fatal("reached ratio resumed")
	}
	if err = finished.Start(); err != nil {
		t.Fatal(err)
	}
	waitRatioPaused(t, finished)
	if requests.Load() != before {
		t.Fatal("reached ratio contacted tracker")
	}
	if err = finished.SetSeedingPolicy(time.Now().Add(time.Minute), 150); err == nil {
		t.Fatal("accepted already reached ratio")
	}
	prior := finished.SeedingState()
	if err = reached.db.Close(); err != nil {
		t.Fatal(err)
	}
	if err = finished.SetSeedingPolicy(time.Now().Add(time.Minute), 200); err == nil {
		t.Fatal("accepted failed ratio save")
	}
	if finished.SeedingState() != prior {
		t.Fatal("failed ratio save changed policy")
	}
	if err = finished.SetSeedingPolicy(time.Time{}, 0); err == nil {
		t.Fatal("failed stop save hidden")
	}
	if state = finished.SeedingState(); state.RatioPercent != 0 || state.TargetBytes != 0 || state.Until != 0 || state.Active {
		t.Fatal("failed save retained runtime sharing", state)
	}
}

func waitRatioPaused(t *testing.T, tor *Torrent) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !tor.SeedingState().Ready {
		if time.Now().After(deadline) {
			t.Fatal("torrent did not verify and pause")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestInvalidSeedingRatioStateIsRetained(t *testing.T) {
	db, err := bbolt.Open(filepath.Join(t.TempDir(), "ratio.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := &Session{db: db}
	for _, tc := range []struct{ deadline, ratio string }{{"0", "100"}, {"", "100"}, {"100", "-1"}, {"100", "10001"}, {"100", "NaN"}, {"100", "100000"}} {
		err = db.Update(func(tx *bbolt.Tx) error {
			root, err := tx.CreateBucketIfNotExists(torrentsBucket)
			if err != nil {
				return err
			}
			b, err := root.CreateBucketIfNotExists([]byte("fixture"))
			if err != nil {
				return err
			}
			if err = b.Put(seedDeadlineKey, []byte(tc.deadline)); err != nil {
				return err
			}
			return b.Put(seedRatioKey, []byte(tc.ratio))
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err = s.readSeedingPolicy("fixture"); err == nil {
			t.Fatalf("accepted invalid saved ratio %+v", tc)
		}
		err = db.View(func(tx *bbolt.Tx) error {
			b := tx.Bucket(torrentsBucket).Bucket([]byte("fixture"))
			if string(b.Get(seedDeadlineKey)) != tc.deadline || string(b.Get(seedRatioKey)) != tc.ratio {
				t.Fatal("invalid original was changed")
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
