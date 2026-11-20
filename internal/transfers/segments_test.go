package transfers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func segmentPayload() []byte {
	return bytes.Repeat([]byte("original independently ranged fixture\n"), 262144)
}

func segmentFixture(t *testing.T, payload []byte, mode string, slow bool) (*httptest.Server, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	var ranged, active, peak atomic.Int32
	var changed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := payload
		etag := `"original"`
		if changed.Load() {
			body = bytes.Repeat([]byte{'R'}, len(payload))
			etag = `"replacement"`
		}
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("ETag", etag)
		if mode == "weak" {
			w.Header().Set("ETag", `W/"weak"`)
		}
		start, end := int64(0), int64(len(body)-1)
		if header := r.Header.Get("Range"); header != "" {
			ranged.Add(1)
			if mode == "ignore" || mode == "change" {
				if mode == "change" {
					changed.Store(true)
					body = bytes.Repeat([]byte{'R'}, len(payload))
					w.Header().Set("ETag", `"replacement"`)
				}
				w.Header().Set("Content-Length", strconv.Itoa(len(body)))
				_, _ = w.Write(body)
				return
			}
			if _, err := fmt.Sscanf(header, "bytes=%d-%d", &start, &end); err != nil || start < 0 || end >= int64(len(body)) || end < start {
				w.WriteHeader(416)
				return
			}
			if r.Header.Get("If-Range") != etag {
				t.Error("range did not retain its strong validator")
			}
			headerEnd := end
			if mode == "malformed" {
				headerEnd++
			}
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, headerEnd, len(body)))
			if mode != "chunked" && mode != "overflow" {
				w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
			}
			w.WriteHeader(206)
			if mode == "stall" {
				w.(http.Flusher).Flush()
				<-r.Context().Done()
				return
			}
			count := active.Add(1)
			defer active.Add(-1)
			for {
				old := peak.Load()
				if count <= old || peak.CompareAndSwap(old, count) {
					break
				}
			}
			if mode == "truncate" {
				_, _ = w.Write(body[start : start+17])
				return
			}
		} else {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		}
		for offset := start; offset <= end; {
			n := min(int64(8192), end-offset+1)
			if _, err := w.Write(body[offset : offset+n]); err != nil {
				return
			}
			offset += n
			w.(http.Flusher).Flush()
			if slow {
				time.Sleep(2 * time.Millisecond)
			}
		}
		if r.Header.Get("Range") != "" && mode == "overflow" {
			w.(http.Flusher).Flush()
			time.Sleep(100 * time.Millisecond)
			_, _ = w.Write([]byte{'x'})
		}
	}))
	t.Cleanup(server.Close)
	return server, &ranged, &peak
}

func TestSegmentedRealParallelTransferAndIntegrity(t *testing.T) {
	payload := segmentPayload()
	server, ranges, peak := segmentFixture(t, payload, "normal", true)
	m, cfg := openTest(t, nil)
	id, err := m.Add(server.URL + "/parallel.bin")
	if err != nil {
		t.Fatal(err)
	}
	job := complete(t, m, id)
	if ranges.Load() != 3 || peak.Load() < 2 || len(job.Segments) != 3 || !bytes.Equal(content(t, cfg, job), payload) {
		t.Fatal("parallel ranges or exact bytes failed", ranges.Load(), peak.Load())
	}
	if !validSegments(job) || job.Committed != int64(len(payload)) {
		t.Fatal("completed checkpoint does not cover all bytes")
	}
}

func TestSingleConnectionOptOut(t *testing.T) {
	payload := segmentPayload()
	server, ranges, _ := segmentFixture(t, payload, "normal", false)
	m, cfg := openTest(t, nil)
	id, err := m.AddWithOptions(server.URL+"/single.bin", AddOptions{SingleStream: true})
	if err != nil {
		t.Fatal(err)
	}
	job := complete(t, m, id)
	if ranges.Load() != 0 || len(job.Segments) != 0 || !job.SingleStream || !bytes.Equal(content(t, cfg, job), payload) {
		t.Fatal("single-connection choice was ignored")
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if !reopened.Jobs()[0].SingleStream {
		t.Fatal("single-connection choice did not persist")
	}
}

func TestSegmentedPauseRestartUsesCheckpointedRanges(t *testing.T) {
	payload := segmentPayload()
	server, ranges, _ := segmentFixture(t, payload, "normal", true)
	m, cfg := openTest(t, nil)
	id, err := m.Add(server.URL + "/resume.bin")
	if err != nil {
		t.Fatal(err)
	}
	await(t, m, id, func(j Job) bool { return len(j.Segments) == 3 && j.Completed >= 128<<10 })
	if err = m.Control(id, "pause"); err != nil {
		t.Fatal(err)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	job := m.Jobs()[0]
	if job.Committed == 0 || job.Committed >= job.Total {
		t.Fatal("pause did not save partial ranges")
	}
	for _, s := range job.Segments {
		if s.Done > 0 {
			if s.Done >= s.End-s.Start+1 {
				t.Fatal("fixture finished before pause")
			}
		}
	}
	file, err := os.OpenFile(filepath.Join(cfg.Downloads, partial(id)), os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if file.Truncate(job.Total+4096) != nil {
		t.Fatal("cannot add synthetic crash tail")
	}
	_ = file.Close()
	restarted, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	copy := restarted.Jobs()
	copy[0].Segments[0].Done = math.MaxInt64
	if !validSegments(restarted.Jobs()[0]) {
		t.Fatal("caller changed internal checkpoints through Jobs")
	}
	if err = restarted.Control(id, "unpause"); err != nil {
		t.Fatal(err)
	}
	final := complete(t, restarted, id)
	if ranges.Load() < 4 || !bytes.Equal(content(t, cfg, final), payload) {
		t.Fatal("restart did not resume exact ranges")
	}
}

func TestSegmentedFallbackAndChangedResource(t *testing.T) {
	for _, mode := range []string{"ignore", "change", "weak"} {
		t.Run(mode, func(t *testing.T) {
			payload := segmentPayload()
			server, ranges, _ := segmentFixture(t, payload, mode, false)
			m, cfg := openTest(t, nil)
			id, err := m.Add(server.URL + "/fallback.bin")
			if err != nil {
				t.Fatal(err)
			}
			job := complete(t, m, id)
			expected := payload
			if mode == "change" {
				expected = bytes.Repeat([]byte{'R'}, len(payload))
			}
			if !bytes.Equal(content(t, cfg, job), expected) || len(job.Segments) != 0 {
				t.Fatal("fallback mixed versions or retained holes")
			}
			if mode == "weak" && ranges.Load() != 0 || ranges.Load() > 3 {
				t.Fatal("unsafe validator used or unbounded range retry")
			}
		})
	}
}

func TestMalformedOrTruncatedSegmentsNeverFinalize(t *testing.T) {
	for _, mode := range []string{"malformed", "truncate", "overflow"} {
		t.Run(mode, func(t *testing.T) {
			payload := segmentPayload()
			server, _, _ := segmentFixture(t, payload, mode, false)
			m, cfg := openTest(t, nil)
			id, err := m.Add(server.URL + "/broken.bin")
			if err != nil {
				t.Fatal(err)
			}
			job := await(t, m, id, func(j Job) bool { return j.Status == "error" })
			if job.Error == "" {
				t.Fatal("broken partial marked complete")
			}
			if _, err = os.Stat(filepath.Join(cfg.Downloads, "broken.bin")); !os.IsNotExist(err) {
				t.Fatal("incomplete segments published a destination file")
			}
			if mode == "overflow" {
				if job.Committed >= job.Total {
					t.Fatal("unverified EOF saved a fully complete checkpoint")
				}
				if err = m.Control(id, "unpause"); err != nil {
					t.Fatal(err)
				}
				await(t, m, id, func(j Job) bool { return j.Status == "error" })
				if _, err = os.Stat(filepath.Join(cfg.Downloads, "broken.bin")); !os.IsNotExist(err) {
					t.Fatal("resume bypassed malformed response validation")
				}
			}
		})
	}
}

func TestSegmentedChunkedRangeAndStall(t *testing.T) {
	payload := segmentPayload()
	t.Run("chunked", func(t *testing.T) {
		server, _, _ := segmentFixture(t, payload, "chunked", false)
		m, cfg := openTest(t, nil)
		id, err := m.Add(server.URL + "/chunked.bin")
		if err != nil {
			t.Fatal(err)
		}
		job := complete(t, m, id)
		if !bytes.Equal(content(t, cfg, job), payload) {
			t.Fatal("chunked ranges were not assembled exactly")
		}
	})
	t.Run("stall", func(t *testing.T) {
		server, _, _ := segmentFixture(t, payload, "stall", false)
		m, err := Open(Config{DataDir: t.TempDir(), Downloads: t.TempDir(), StallTimeout: 150 * time.Millisecond})
		if err != nil {
			t.Fatal(err)
		}
		defer m.Close()
		id, err := m.Add(server.URL + "/stalled.bin")
		if err != nil {
			t.Fatal(err)
		}
		await(t, m, id, func(j Job) bool { return j.Status == "error" })
	})
}

func TestSegmentCheckpointValidationAndOverflowBoundaries(t *testing.T) {
	job := Job{Total: math.MaxInt64, ETag: `"strong"`, Segments: makeSegments(math.MaxInt64)}
	if !validSegments(job) {
		t.Fatal("segment arithmetic overflowed")
	}
	job.Segments[0].Done = job.Segments[0].End + 1
	job.Committed = job.Segments[0].Done
	job.Completed = job.Committed
	if !validSegments(job) {
		t.Fatal("valid upper-bound offsets rejected")
	}
	job.Segments[1].Start++
	if validSegments(job) {
		t.Fatal("checkpoint hole accepted")
	}
	m, cfg := openTest(t, nil)
	id, err := m.AddPlanned("http://example.invalid/never", time.Now().Add(time.Hour).UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(cfg.DataDir, "http-transfers.json"))
	if err != nil {
		t.Fatal(err)
	}
	var state state
	if json.Unmarshal(raw, &state) != nil {
		t.Fatal("state parse")
	}
	state.Jobs[0].Total = 9 << 20
	state.Jobs[0].ETag = `"valid"`
	state.Jobs[0].Segments = []Segment{{Start: 1, End: 9<<20 - 1}}
	raw, _ = json.Marshal(state)
	if os.WriteFile(filepath.Join(cfg.DataDir, "http-transfers.json"), raw, 0600) != nil {
		t.Fatal("cannot prepare malformed checkpoint")
	}
	if reopened, err := Open(cfg); err == nil {
		reopened.Close()
		t.Fatal("malformed segment registry accepted", id)
	}
	after, _ := os.ReadFile(filepath.Join(cfg.DataDir, "http-transfers.json"))
	if !bytes.Equal(after, raw) {
		t.Fatal("invalid checkpoint bytes were rewritten")
	}
}

func TestSegmentedSharedRateLimitAndConcurrentPause(t *testing.T) {
	payload := segmentPayload()
	server, _, _ := segmentFixture(t, payload, "normal", true)
	m, _ := openTest(t, nil)
	if err := m.SetLimit(32768); err != nil {
		t.Fatal(err)
	}
	id, err := m.Add(server.URL + "/limited.bin")
	if err != nil {
		t.Fatal(err)
	}
	await(t, m, id, func(j Job) bool { return len(j.Segments) == 3 })
	time.Sleep(350 * time.Millisecond)
	job := m.Jobs()[0]
	if job.Completed > 65536 {
		t.Fatal("each segment got a separate speed budget", job.Completed)
	}
	var group sync.WaitGroup
	for range 4 {
		group.Add(1)
		go func() { defer group.Done(); _ = m.Control(id, "pause"); _ = m.Jobs() }()
	}
	group.Wait()
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	if !validSegments(m.Jobs()[0]) {
		t.Fatal("concurrent pause corrupted checkpoints")
	}
}
