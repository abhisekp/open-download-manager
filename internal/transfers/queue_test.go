package transfers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestQueueConcurrencyAndHostFairness(t *testing.T) {
	starts := make(chan string, 8)
	release := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		starts <- r.URL.Path
		select {
		case <-release:
			_, _ = w.Write([]byte("done"))
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(s.Close)
	m, _ := openTest(t, nil)
	if err := m.SetQueue(QueueSettings{2, 1}); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{s.URL + "/host-one-first", s.URL + "/host-one-second", strings.Replace(s.URL, "127.0.0.1", "localhost", 1) + "/host-two"} {
		if _, err := m.Add(raw); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	for range 2 {
		select {
		case path := <-starts:
			seen[path] = true
		case <-time.After(5 * time.Second):
			t.Fatal("host limit blocked an independent host")
		}
	}
	if !seen["/host-one-first"] || !seen["/host-two"] {
		t.Fatal("per-host limit was ignored", seen)
	}
	select {
	case <-starts:
		t.Fatal("concurrency or per-host limit exceeded")
	case <-time.After(200 * time.Millisecond):
	}
	if err := m.SetQueue(QueueSettings{1, 1}); err != nil {
		t.Fatal(err)
	}
	// Lowering limits does not cancel either active request.
	for _, j := range m.Jobs() {
		if seen["/"+j.Name] && j.Status != "active" {
			t.Fatal("configuration preempted a running request")
		}
	}
	close(release)
	select {
	case path := <-starts:
		if path != "/host-one-second" {
			t.Fatal("unexpected queued request")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waiting request did not start")
	}
	for _, j := range m.Jobs() {
		complete(t, m, j.ID)
	}
}

func TestScheduledRequestIsPersistedAndNeverRunsEarly(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte("scheduled original bytes"))
	}))
	defer server.Close()
	m, cfg := openTest(t, nil)
	start := time.Now().Add(1200 * time.Millisecond).UnixMilli()
	id, err := m.AddPlanned(server.URL+"/later", start)
	if err != nil {
		t.Fatal(err)
	}
	if m.Jobs()[0].Status != "scheduled" {
		t.Fatal("schedule not retained")
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	m2, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m2.Close()
	time.Sleep(250 * time.Millisecond)
	if requests.Load() != 0 {
		t.Fatal("scheduled URL contacted before chosen start")
	}
	job := complete(t, m2, id)
	if string(content(t, cfg, job)) != "scheduled original bytes" || requests.Load() != 1 || time.Now().UnixMilli() < start {
		t.Fatal("scheduled transfer integrity or timing failed")
	}
}

func TestPausedScheduleAndResumeNow(t *testing.T) {
	var requests atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); _, _ = w.Write([]byte("resume")) }))
	defer s.Close()
	m, _ := openTest(t, nil)
	id, err := m.AddPlanned(s.URL, time.Now().Add(350*time.Millisecond).UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Control(id, "pause"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	if requests.Load() != 0 {
		t.Fatal("paused schedule still fired")
	}
	if err = m.Control(id, "unpause"); err != nil {
		t.Fatal(err)
	}
	if complete(t, m, id).StartAt != 0 {
		t.Fatal("explicit resume did not clear schedule")
	}
}

func TestOverdueScheduleSurvivesClosedAppAndQueueOrder(t *testing.T) {
	started := make(chan string, 4)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- r.URL.Path
		_, _ = w.Write([]byte(r.URL.Path))
	}))
	defer s.Close()
	m, cfg := openTest(t, nil)
	if err := m.SetQueue(QueueSettings{1, 1}); err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, path := range []string{"/first", "/second"} {
		id, err := m.AddPlanned(s.URL+path, time.Now().Add(350*time.Millisecond).UnixMilli())
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := m.Move(ids[1], "first"); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(450 * time.Millisecond)
	m2, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m2.Close()
	if m2.QueueSettings() != (QueueSettings{1, 1}) {
		t.Fatal("settings not persisted")
	}
	complete(t, m2, ids[0])
	complete(t, m2, ids[1])
	if <-started != "/second" || <-started != "/first" {
		t.Fatal("overdue jobs ignored persisted order")
	}
}

func TestQueueBoundsAndRollback(t *testing.T) {
	m, _ := openTest(t, nil)
	for _, q := range []QueueSettings{{0, 1}, {9, 1}, {2, 3}, {3, 4}} {
		if m.SetQueue(q) == nil {
			t.Fatal("invalid limits accepted")
		}
	}
	for _, when := range []int64{-1, time.Now().Add(-time.Second).UnixMilli(), time.Now().AddDate(2, 0, 0).UnixMilli()} {
		if _, err := m.AddPlanned("http://example.invalid/file", when); err == nil {
			t.Fatal("invalid schedule accepted")
		}
	}
	ids := []string{}
	for range 2 {
		id, err := m.AddPlanned("http://example.invalid/later", time.Now().Add(time.Hour).UnixMilli())
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if m.Move(ids[0], "arbitrary") == nil || m.Move("missing", "first") == nil {
		t.Fatal("invalid reorder accepted")
	}
	old := m.registry
	m.registry = filepath.Join(t.TempDir(), "missing", "state.json")
	before := m.Jobs()
	settings := m.QueueSettings()
	if m.SetQueue(QueueSettings{1, 1}) == nil || m.Move(ids[1], "first") == nil {
		t.Fatal("failed persistence reported success")
	}
	if m.QueueSettings() != settings || m.Jobs()[0].ID != before[0].ID || m.Jobs()[0].Order != before[0].Order {
		t.Fatal("failed write changed live configuration")
	}
	if _, err := m.AddPlanned("http://example.invalid/new", time.Now().Add(time.Hour).UnixMilli()); err == nil {
		t.Fatal("failed schedule write accepted")
	}
	if len(m.Jobs()) != 2 {
		t.Fatal("failed schedule left runnable job")
	}
	m.registry = old
	if info, err := os.Stat(m.registry); err != nil || info.Size() == 0 {
		t.Fatal("prior registry was lost")
	}
}
