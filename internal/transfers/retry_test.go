package transfers

import (
	"bytes"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func retryManager(t *testing.T) (*Manager, Config) {
	t.Helper()
	cfg := Config{DataDir: filepath.Join(testDir(t), "data"), Downloads: filepath.Join(testDir(t), "files"), DefaultRetryLimit: 3, RetryBase: 100 * time.Millisecond}
	m, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	})
	return m, cfg
}

func TestAutomaticRetryBudgetAndQueueFairness(t *testing.T) {
	m, cfg := retryManager(t)
	if err := m.SetQueue(QueueSettings{Concurrent: 1, PerHost: 1}); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/retry.bin" {
			calls.Add(1)
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(503)
			return
		}
		_, _ = w.Write([]byte("second job is not blocked"))
	}))
	defer server.Close()
	limit := 2
	id, err := m.AddWithOptions(server.URL+"/retry.bin", AddOptions{RetryLimit: &limit})
	if err != nil {
		t.Fatal(err)
	}
	row := await(t, m, id, func(j Job) bool { return j.Status == "retrying" })
	if row.RetryCount != 1 || row.RetryAt < time.Now().Add(700*time.Millisecond).UnixMilli() {
		t.Fatal("server delay/count not honored")
	}
	other, err := m.Add(server.URL + "/other.bin")
	if err != nil {
		t.Fatal(err)
	}
	complete(t, m, other)
	if calls.Load() != 1 {
		t.Fatal("retried before server delay or blocked the other queue entry")
	}
	row = await(t, m, id, func(j Job) bool { return j.Status == "error" })
	if calls.Load() != 3 || row.RetryCount != 2 || row.RetryAt != 0 {
		t.Fatal("retry budget was not bounded", calls.Load(), row)
	}
	if _, err := os.Stat(filepath.Join(cfg.Downloads, row.Name)); !os.IsNotExist(err) {
		t.Fatal("failed HTTP response became a completed file")
	}
}

func TestAutomaticRetryResumesOnlyValidatedPartialBytes(t *testing.T) {
	m, cfg := retryManager(t)
	body := bytes.Repeat([]byte("retry-safe-payload\n"), 8192)
	var calls atomic.Int32
	var resumed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := calls.Add(1)
		w.Header().Set("ETag", `"retry-v1"`)
		if attempt == 1 {
			w.Header().Set("Content-Length", fmt.Sprint(len(body)))
			_, _ = w.Write(body[:32768])
			w.(http.Flusher).Flush()
			connection, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = connection.Close()
			}
			return
		}
		if r.Header.Get("Range") != "bytes=32768-" || r.Header.Get("If-Range") != `"retry-v1"` {
			w.WriteHeader(400)
			return
		}
		resumed.Store(true)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes 32768-%d/%d", len(body)-1, len(body)))
		w.Header().Set("Content-Length", fmt.Sprint(len(body)-32768))
		w.WriteHeader(206)
		_, _ = w.Write(body[32768:])
	}))
	defer server.Close()
	id, err := m.Add(server.URL + "/resumed.bin")
	if err != nil {
		t.Fatal(err)
	}
	row := complete(t, m, id)
	actual, err := os.ReadFile(filepath.Join(cfg.Downloads, row.Name))
	if err != nil || !bytes.Equal(actual, body) || !resumed.Load() || calls.Load() != 2 || row.RetryCount != 1 {
		t.Fatal("retry did not preserve validator-safe bytes", err, row)
	}
}

func TestRetryRestartAndPauseCancellation(t *testing.T) {
	m, cfg := retryManager(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			return
		}
		_, _ = w.Write([]byte("restored retry"))
	}))
	defer server.Close()
	id, err := m.Add(server.URL + "/restart.bin")
	if err != nil {
		t.Fatal(err)
	}
	waiting := await(t, m, id, func(j Job) bool { return j.Status == "retrying" })
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	initial := restored.Jobs()[0]
	if initial.RetryAt != waiting.RetryAt || initial.RetryCount != 1 {
		t.Fatal("retry intent changed after restart")
	}
	row := complete(t, restored, id)
	if row.RetryCount != 1 || calls.Load() != 2 {
		t.Fatal("restored retry repeated or lost its budget")
	}
	// A separate always-busy origin must stop receiving requests after pause.
	var pausedCalls atomic.Int32
	busy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pausedCalls.Add(1)
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(503)
	}))
	defer busy.Close()
	paused, err := restored.Add(busy.URL + "/paused.bin")
	if err != nil {
		t.Fatal(err)
	}
	await(t, restored, paused, func(j Job) bool { return j.Status == "retrying" })
	if err := restored.Control(paused, "pause"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1200 * time.Millisecond)
	row = await(t, restored, paused, func(j Job) bool { return j.Status == "paused" })
	if pausedCalls.Load() != 1 || row.RetryAt != 0 {
		t.Fatal("paused retry contacted its source")
	}
}

func TestSegmentedTemporaryFailureRetriesValidatedRanges(t *testing.T) {
	m, cfg := retryManager(t)
	body := segmentPayload()
	original, _, _ := segmentFixture(t, body, "", false)
	var failed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" && failed.CompareAndSwap(false, true) {
			w.WriteHeader(503)
			return
		}
		original.Config.Handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	id, err := m.Add(server.URL + "/ranges.bin")
	if err != nil {
		t.Fatal(err)
	}
	row := complete(t, m, id)
	actual, err := os.ReadFile(filepath.Join(cfg.Downloads, row.Name))
	if err != nil || !bytes.Equal(actual, body) || row.RetryCount != 1 || len(row.Segments) != 3 {
		t.Fatal("segment retry mixed or lost bytes", err, row)
	}
}

func TestRetryPersistenceFailureCannotScheduleNetworkWork(t *testing.T) {
	m, _ := retryManager(t)
	id, err := m.AddPlanned("https://example.invalid/no-network", time.Now().Add(time.Hour).UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	registry := m.registry
	m.registry = filepath.Join(testDir(t), "absent", "registry.json")
	job := m.jobs[id]
	job.Status = "active"
	accepted := m.retry(job, &transientError{message: "fixture temporary failure"})
	bad := !accepted || job.Status != "error" || job.RetryAt != 0
	m.registry = registry
	err = m.save()
	m.mu.Unlock()
	if bad || err != nil {
		t.Fatal("failed journal write left an executable retry", err)
	}
}

func TestRetrySafetyBoundaries(t *testing.T) {
	m, _ := retryManager(t)
	for _, status := range []int{401, 403, 404, 416} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(status) }))
			defer s.Close()
			id, err := m.Add(s.URL + "/denied.bin")
			if err != nil {
				t.Fatal(err)
			}
			row := await(t, m, id, func(j Job) bool { return j.Status == "error" })
			if calls.Load() != 1 || row.RetryCount != 0 {
				t.Fatal("permanent response retried")
			}
		})
	}
	for _, err := range []error{&url.Error{Op: "Get", URL: "https://secret.invalid/?private=redacted", Err: errors.New("HTTPS downgrade redirect rejected")}, x509.UnknownAuthorityError{}, &net.OpError{Op: "dial", Net: "tcp", Err: &net.DNSError{Err: "not found", Name: "private.invalid", IsNotFound: true}}} {
		var transient *transientError
		if errors.As(connectionError(err), &transient) || strings.Contains(connectionError(err).Error(), "private") {
			t.Fatal("security failure retried or URL leaked")
		}
	}
	for _, header := range []string{"901", strings.Repeat("9", 20), strings.Repeat("9", 100), time.Now().Add(time.Hour).UTC().Format(http.TimeFormat)} {
		err := responseError(&http.Response{StatusCode: 503, Header: http.Header{"Retry-After": []string{header}}})
		var temporary *transientError
		if errors.As(err, &temporary) {
			t.Fatal("long server delay was shortened", header)
		}
	}
	for _, limit := range []int{-1, 6} {
		if _, err := m.AddWithOptions("https://example.invalid/file", AddOptions{RetryLimit: &limit}); err == nil {
			t.Fatal("invalid retry limit accepted")
		}
	}
	zero := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer s.Close()
	id, err := m.AddWithOptions(s.URL+"/manual.bin", AddOptions{RetryLimit: &zero})
	if err != nil {
		t.Fatal(err)
	}
	if row := await(t, m, id, func(j Job) bool { return j.Status == "error" }); row.RetryCount != 0 {
		t.Fatal("retry opt-out ignored")
	}
}
