package transfers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func openTest(t *testing.T, client *http.Client) (*Manager, Config) {
	t.Helper()
	config := Config{DataDir: filepath.Join(t.TempDir(), "data"), Downloads: filepath.Join(t.TempDir(), "files"), Client: client}
	m, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := m.Close(); err != nil {
			t.Error("manager cleanup:", err)
		}
	})
	return m, config
}
func await(t *testing.T, m *Manager, id string, predicate func(Job) bool) Job {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		for _, job := range m.Jobs() {
			if job.ID == id && predicate(job) {
				return job
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for transfer %s: %+v", id, m.Jobs())
	return Job{}
}
func complete(t *testing.T, m *Manager, id string) Job {
	t.Helper()
	job := await(t, m, id, func(j Job) bool { return j.Status == "complete" || j.Status == "error" })
	if job.Status != "complete" {
		t.Fatal(job.Error)
	}
	return job
}
func content(t *testing.T, config Config, job Job) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(config.Downloads, job.Name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func ranged(payload []byte, etag string, w http.ResponseWriter, r *http.Request, slow bool) {
	offset := 0
	if header := r.Header.Get("Range"); header != "" {
		_, _ = fmt.Sscanf(header, "bytes=%d-", &offset)
	}
	if offset > len(payload) {
		w.WriteHeader(416)
		return
	}
	body := payload[offset:]
	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	if offset > 0 {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, len(payload)-1, len(payload)))
		w.WriteHeader(206)
	}
	for len(body) > 0 {
		n := min(len(body), 8192)
		if _, err := w.Write(body[:n]); err != nil {
			return
		}
		body = body[n:]
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		if slow {
			time.Sleep(2 * time.Millisecond)
		}
	}
}

func TestHTTPSRedirectCollisionAndEmptyFile(t *testing.T) {
	payload := bytes.Repeat([]byte("original TLS fixture"), 1024)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect.bin" {
			http.Redirect(w, r, "/file.bin", 302)
			return
		}
		if r.URL.Path == "/empty" {
			w.Header().Set("Content-Length", "0")
			return
		}
		ranged(payload, `"one"`, w, r, false)
	}))
	defer server.Close()
	m, config := openTest(t, server.Client())
	existing := filepath.Join(config.Downloads, "redirect.bin")
	if err := os.WriteFile(existing, []byte("keep existing"), 0600); err != nil {
		t.Fatal(err)
	}
	id, err := m.Add(server.URL + "/redirect.bin")
	if err != nil {
		t.Fatal(err)
	}
	job := complete(t, m, id)
	if job.Name != "redirect (1).bin" || !bytes.Equal(content(t, config, job), payload) {
		t.Fatal("redirect/collision content mismatch", job)
	}
	if data, _ := os.ReadFile(existing); string(data) != "keep existing" {
		t.Fatal("existing file overwritten")
	}
	id, err = m.Add(server.URL + "/empty")
	if err != nil {
		t.Fatal(err)
	}
	job = complete(t, m, id)
	if job.Total != 0 || len(content(t, config, job)) != 0 {
		t.Fatal("empty transfer incorrect")
	}
}

func TestPauseResumeAfterRestartUsesValidators(t *testing.T) {
	payload := bytes.Repeat([]byte("range-resume-original"), 65536)
	var resumed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			if r.Header.Get("If-Range") != `"stable"` {
				t.Error("missing If-Range validator")
			}
			resumed.Store(true)
		}
		ranged(payload, `"stable"`, w, r, true)
	}))
	defer server.Close()
	m, config := openTest(t, nil)
	id, err := m.Add(server.URL + "/file.bin")
	if err != nil {
		t.Fatal(err)
	}
	await(t, m, id, func(j Job) bool { return j.Completed > 65536 && j.Status == "active" })
	if err = m.Control(id, "pause"); err != nil {
		t.Fatal(err)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if restarted.Jobs()[0].Status != "paused" {
		t.Fatal("restart silently resumed the job")
	}
	if err = restarted.Control(id, "unpause"); err != nil {
		t.Fatal(err)
	}
	job := complete(t, restarted, id)
	if !resumed.Load() || !bytes.Equal(content(t, config, job), payload) {
		t.Fatal("resumed content differs")
	}
}

func TestChangedOrIgnoredResumeNeverMixesResources(t *testing.T) {
	for _, mode := range []string{"ignored-range", "changed-etag"} {
		t.Run(mode, func(t *testing.T) {
			before := bytes.Repeat([]byte("A"), 512<<10)
			after := bytes.Repeat([]byte("B"), len(before))
			var changed atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !changed.Load() {
					ranged(before, `"old"`, w, r, true)
					return
				}
				if mode == "ignored-range" {
					r.Header.Del("Range")
				}
				ranged(after, `"new"`, w, r, false)
			}))
			defer server.Close()
			m, config := openTest(t, nil)
			id, _ := m.Add(server.URL + "/changing.bin")
			await(t, m, id, func(j Job) bool { return j.Status == "active" && j.Completed > 32768 })
			if err := m.Control(id, "pause"); err != nil {
				t.Fatal(err)
			}
			if err := m.Close(); err != nil {
				t.Fatal(err)
			}
			changed.Store(true)
			restarted, err := Open(config)
			if err != nil {
				t.Fatal(err)
			}
			defer restarted.Close()
			if err = restarted.Control(id, "unpause"); err != nil {
				t.Fatal(err)
			}
			job := complete(t, restarted, id)
			if !bytes.Equal(content(t, config, job), after) {
				t.Fatal("mixed old/new bytes during resume")
			}
		})
	}
}

func TestTruncatedResponseCanResumeWithoutUncommittedTail(t *testing.T) {
	payload := bytes.Repeat([]byte("durable fixture"), 32768)
	var first atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !first.Swap(true) {
			w.Header().Set("ETag", `"same"`)
			w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
			_, _ = w.Write(payload[:len(payload)/2])
			return
		}
		ranged(payload, `"same"`, w, r, false)
	}))
	defer server.Close()
	m, config := openTest(t, nil)
	id, _ := m.Add(server.URL + "/interrupted.bin")
	await(t, m, id, func(j Job) bool { return j.Status == "error" })
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(config.Downloads, partial(id)), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write(bytes.Repeat([]byte("uncommitted junk"), 1024))
	_ = f.Close()
	restarted, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if err = restarted.Control(id, "unpause"); err != nil {
		t.Fatal(err)
	}
	job := complete(t, restarted, id)
	if !bytes.Equal(content(t, config, job), payload) {
		t.Fatal("uncommitted crash tail was appended into the resource")
	}
}

func TestRateLimitPersists(t *testing.T) {
	payload := bytes.Repeat([]byte("r"), 65536)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { ranged(payload, `"rate"`, w, r, false) }))
	defer server.Close()
	m, config := openTest(t, nil)
	if err := m.SetLimit(32768); err != nil {
		t.Fatal(err)
	}
	for iteration := 0; iteration < 2; iteration++ {
		start := time.Now()
		id, err := m.Add(server.URL + "/rate.bin")
		if err != nil {
			t.Fatal(err)
		}
		complete(t, m, id)
		if time.Since(start) < 800*time.Millisecond {
			t.Fatal("configured rate limit was not enforced")
		}
		if iteration == 0 {
			if err = m.Close(); err != nil {
				t.Fatal(err)
			}
			m, err = Open(config)
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
		}
	}
}

func TestQueueBoundAndStalledBodyCancellation(t *testing.T) {
	var active, peak atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := active.Add(1)
		defer active.Add(-1)
		for {
			previous := peak.Load()
			if count <= previous || peak.CompareAndSwap(previous, count) {
				break
			}
		}
		w.Header().Set("Content-Length", "1")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	m, config := openTest(t, nil)
	for i := 0; i < 5; i++ {
		if _, err := m.Add(server.URL + "/queued.bin"); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for peak.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if peak.Load() != 3 {
		t.Fatal("queue did not start three bounded workers", peak.Load())
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	config.StallTimeout = 100 * time.Millisecond
	stalled, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	defer stalled.Close()
	id, err := stalled.Add(server.URL + "/stalled.bin")
	if err != nil {
		t.Fatal(err)
	}
	job := await(t, stalled, id, func(j Job) bool { return j.Status == "error" })
	if !strings.Contains(job.Error, "stalled") {
		t.Fatal("unexpected stall result", job.Error)
	}
}

func TestTLSVerificationAndHTTPSDowngradeAreEnforced(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("must not receive downgraded download"))
	}))
	defer plain.Close()
	tls := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, plain.URL+"/file", 302) }))
	defer tls.Close()
	untrusted, _ := openTest(t, nil)
	id, err := untrusted.Add(tls.URL + "/file")
	if err != nil {
		t.Fatal(err)
	}
	job := await(t, untrusted, id, func(j Job) bool { return j.Status == "error" })
	if !strings.Contains(job.Error, "TLS certificate") {
		t.Fatal("untrusted certificate accepted", job)
	}
	trusted, _ := openTest(t, tls.Client())
	id, err = trusted.Add(tls.URL + "/file")
	if err != nil {
		t.Fatal(err)
	}
	job = await(t, trusted, id, func(j Job) bool { return j.Status == "error" })
	if !strings.Contains(job.Error, "connection failed") {
		t.Fatal("HTTPS downgrade not rejected", job)
	}
	if _, err = trusted.Add("https://user:password@example.com/file"); err == nil {
		t.Fatal("embedded credentials accepted without secure storage")
	}
}

func TestCorruptOrUnsafeRegistryFailsWithoutTouchingUserFiles(t *testing.T) {
	dir := t.TempDir()
	downloads := filepath.Join(dir, "files")
	data := filepath.Join(dir, "data")
	if err := os.MkdirAll(data, 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(dir, "user.txt")
	if err := os.WriteFile(sentinel, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	unsafe := `{"version":1,"limit":0,"jobs":[{"id":"0123456789abcdef","url":"https://example.com/file","name":"../user.txt","status":"finalizing","total":0,"completed":0,"committed":0,"etag":"","last_modified":"","error":"","created":1}]}`
	if err := os.WriteFile(filepath.Join(data, "http-transfers.json"), []byte(unsafe), 0600); err != nil {
		t.Fatal(err)
	}
	if m, err := Open(Config{DataDir: data, Downloads: downloads}); err == nil {
		_ = m.Close()
		t.Fatal("unsafe registry accepted")
	}
	if body, _ := os.ReadFile(sentinel); string(body) != "preserve" {
		t.Fatal("user file altered during recovery")
	}
}

func TestLongCollisionFilenameSurvivesRestart(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("fixture")) }))
	defer server.Close()
	m, config := openTest(t, nil)
	name := strings.Repeat("a", 156) + ".bin"
	if err := os.WriteFile(filepath.Join(config.Downloads, name), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	id, err := m.Add(server.URL + "/" + name)
	if err != nil {
		t.Fatal(err)
	}
	job := complete(t, m, id)
	if len(job.Name) > 160 || job.Name == name {
		t.Fatal("invalid collision name", job.Name)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if string(content(t, config, restarted.Jobs()[0])) != "fixture" {
		t.Fatal("collision output not recovered")
	}
}

func TestFinalizationRecoveryPreservesUnexpectedUserFile(t *testing.T) {
	for _, existing := range []string{"", "unrelated user file"} {
		t.Run(fmt.Sprintf("existing-%d", len(existing)), func(t *testing.T) {
			dir := t.TempDir()
			config := Config{DataDir: filepath.Join(dir, "data"), Downloads: filepath.Join(dir, "files")}
			_ = os.MkdirAll(config.DataDir, 0700)
			_ = os.MkdirAll(config.Downloads, 0700)
			payload := []byte("completed original fixture")
			job := Job{ID: "0123456789abcdef", URL: "https://example.com/file.bin", Name: "file.bin", Status: "finalizing", Total: int64(len(payload)), Completed: int64(len(payload)), Committed: int64(len(payload)), Created: 1}
			body, _ := json.Marshal(state{Version: 1, Jobs: []Job{job}})
			_ = os.WriteFile(filepath.Join(config.DataDir, "http-transfers.json"), body, 0600)
			_ = os.WriteFile(filepath.Join(config.Downloads, partial(job.ID)), payload, 0600)
			_ = os.WriteFile(filepath.Join(config.Downloads, job.Name), []byte(existing), 0600)
			m, err := Open(config)
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			actual := m.Jobs()[0]
			if existing == "" {
				if actual.Status != "complete" || !bytes.Equal(content(t, config, actual), payload) {
					t.Fatal("journal recovery failed", actual)
				}
			} else {
				if actual.Status != "error" || string(content(t, config, actual)) != existing {
					t.Fatal("user file overwritten during recovery", actual)
				}
				if _, err = os.Stat(filepath.Join(config.Downloads, partial(job.ID))); err != nil {
					t.Fatal("partial data removed")
				}
			}
		})
	}
}

func TestMalformedResumeCannotAlterPreviouslyDownloadedBytes(t *testing.T) {
	payload := bytes.Repeat([]byte("correct"), 32768)
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"stable"`)
		if requests.Add(1) == 1 {
			w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
			_, _ = w.Write(payload[:len(payload)/2])
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", len(payload)-1, len(payload)))
		w.WriteHeader(206)
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	m, config := openTest(t, nil)
	id, _ := m.Add(server.URL + "/invalid-range.bin")
	await(t, m, id, func(j Job) bool { return j.Status == "error" })
	before, err := os.ReadFile(filepath.Join(config.Downloads, partial(id)))
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Control(id, "unpause"); err != nil {
		t.Fatal(err)
	}
	job := await(t, m, id, func(j Job) bool { return j.Status == "error" })
	if !strings.Contains(job.Error, "byte range") {
		t.Fatal("malformed range accepted", job)
	}
	after, err := os.ReadFile(filepath.Join(config.Downloads, partial(id)))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("invalid resume changed partial data", err)
	}
}

func TestOversizedChunkedRangeDoesNotWritePastDeclaredSize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", "bytes 0-3/4")
		w.WriteHeader(206)
		w.(http.Flusher).Flush()
		_, _ = w.Write([]byte("oversized response"))
	}))
	defer server.Close()
	m, config := openTest(t, nil)
	id, err := m.Add(server.URL + "/bounded.bin")
	if err != nil {
		t.Fatal(err)
	}
	job := await(t, m, id, func(j Job) bool { return j.Status == "error" })
	if !strings.Contains(job.Error, "exceeded") {
		t.Fatal("oversized range accepted", job)
	}
	info, err := os.Stat(filepath.Join(config.Downloads, partial(id)))
	if err != nil || info.Size() > 4 || job.Completed > 4 {
		t.Fatal("bytes written beyond declared size", err, job.Completed)
	}
}

func TestAttachmentFilenameCannotEscapeDownloadRoot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="../../outside.bin"`)
		_, _ = w.Write([]byte("fixture"))
	}))
	defer server.Close()
	m, config := openTest(t, nil)
	id, err := m.Add(server.URL + "/download")
	if err != nil {
		t.Fatal(err)
	}
	job := complete(t, m, id)
	if job.Name != "outside.bin" || string(content(t, config, job)) != "fixture" {
		t.Fatal("unsafe attachment filename", job)
	}
	if _, err = os.Stat(filepath.Join(filepath.Dir(config.Downloads), "outside.bin")); !os.IsNotExist(err) {
		t.Fatal("attachment escaped download root")
	}
}
