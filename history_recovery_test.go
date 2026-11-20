package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestForgottenHistoryDoesNotReturnFromEngineAfterRestart(t *testing.T) {
	file := filepath.Join(ownedTestDir(t), "history.json")
	h, err := openHistory(file)
	if err != nil {
		t.Fatal(err)
	}
	terminal := []json.RawMessage{json.RawMessage(`{"gid":"same-engine-result","status":"complete"}`)}
	if _, err = h.merge(terminal); err != nil {
		t.Fatal(err)
	}
	if err = h.forget("same-engine-result"); err != nil {
		t.Fatal(err)
	}
	h, err = openHistory(file)
	if err != nil {
		t.Fatal(err)
	}
	got, err := h.merge(terminal)
	if err != nil || len(got) != 0 {
		t.Fatal("forgotten result returned after restart", len(got), err)
	}
}

func TestDelayedStatusAndExportCannotUndoNewerForget(t *testing.T) {
	for _, tc := range []struct {
		name, blocked, route, method, body, result string
		code                                       int
	}{
		{"status", "aria2.tellActive", "status", "GET", "", `[{"gid":"0123456789abcdef","status":"active"}]`, 200},
		{"export", "aria2.tellStopped", "history-export", "POST", `{"approved":true}`, `[{"gid":"0123456789abcdef","status":"complete"}]`, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gid := "0123456789abcdef"
			file := filepath.Join(ownedTestDir(t), "history.json")
			h, err := openHistory(file)
			if err != nil {
				t.Fatal(err)
			}
			terminal := json.RawMessage(`{"gid":"0123456789abcdef","status":"complete"}`)
			if _, err = h.merge([]json.RawMessage{terminal}); err != nil {
				t.Fatal(err)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			defer unblock()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var in struct {
					Method string `json:"method"`
				}
				if json.NewDecoder(r.Body).Decode(&in) != nil {
					http.Error(w, "invalid fixture", 400)
					return
				}
				result := json.RawMessage(`[]`)
				if in.Method == tc.blocked {
					close(entered)
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
					result = json.RawMessage(tc.result)
				} else if in.Method == "aria2.tellStatus" {
					result = terminal
				} else if in.Method == "aria2.removeDownloadResult" {
					result = json.RawMessage(`"OK"`)
				} else if in.Method == "aria2.getGlobalStat" {
					result = json.RawMessage(`{"downloadSpeed":"0","uploadSpeed":"0"}`)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": "odm", "result": result})
			}))
			defer server.Close()
			app := &App{engine: newEngine(server.URL, "owned-rpc-fixture"), history: h, host: "127.0.0.1:9966", token: "owned-history-owner", downloads: ownedTestDir(t)}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			req := httptest.NewRequest(tc.method, "http://"+app.host+"/api/"+tc.route, strings.NewReader(tc.body)).WithContext(ctx)
			req.Header.Set("Authorization", "Bearer "+app.token)
			out := httptest.NewRecorder()
			done := make(chan struct{})
			go func() { defer close(done); app.handler().ServeHTTP(out, req) }()
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal("fixture did not capture engine snapshot")
			}
			forget := httptest.NewRequest("POST", "http://"+app.host+"/api/control", strings.NewReader(`{"gid":"`+gid+`","action":"forget"}`))
			forget.Header.Set("Authorization", "Bearer "+app.token)
			forgot := httptest.NewRecorder()
			app.handler().ServeHTTP(forgot, forget)
			if forgot.Code != 200 {
				t.Fatal("reviewed forget failed", forgot.Code)
			}
			unblock()
			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal("delayed request did not finish")
			}
			if out.Code != tc.code || strings.Contains(out.Body.String(), "History could not be saved") {
				t.Fatal("wrong delayed response", out.Code, out.Body.String())
			}
			if !h.forgotten[gid] || len(h.get(gid)) != 0 {
				t.Fatal("delayed request undid newer forget")
			}
			reopened, err := openHistory(file)
			if err != nil || !reopened.forgotten[gid] {
				t.Fatal("delayed request changed durable suppression", err)
			}
			files, err := os.ReadDir(app.downloads)
			if err != nil || len(files) != 0 {
				t.Fatal("stale export wrote a file", err)
			}
		})
	}
}

func TestStaleEngineSnapshotCannotReleaseForgottenHistory(t *testing.T) {
	file := filepath.Join(ownedTestDir(t), "history.json")
	h, err := openHistory(file)
	if err != nil {
		t.Fatal(err)
	}
	oldSnapshot := h.observation()
	if err = h.forget("stale"); err != nil {
		t.Fatal(err)
	}
	current, err := h.mergeObserved([]json.RawMessage{json.RawMessage(`{"gid":"stale","status":"active"}`)}, oldSnapshot)
	if !errors.Is(err, errHistoryObservationStale) || len(current) != 1 || !h.forgotten["stale"] {
		t.Fatal("stale observation released suppression or hid live progress", err)
	}
	got, err := h.merge([]json.RawMessage{json.RawMessage(`{"gid":"stale","status":"complete"}`)})
	if err != nil || len(got) != 0 {
		t.Fatal("stale snapshot resurrected a forgotten result", err)
	}
	reopened, err := openHistory(file)
	if err != nil || !reopened.forgotten["stale"] {
		t.Fatal("stale snapshot changed durable suppression", err)
	}
	beforeLive := h.observation()
	if _, err = h.merge([]json.RawMessage{json.RawMessage(`{"gid":"stale","status":"active"}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err = h.mergeObserved([]json.RawMessage{json.RawMessage(`{"gid":"stale","status":"complete"}`)}, beforeLive); !errors.Is(err, errHistoryObservationStale) {
		t.Fatal("old terminal snapshot was accepted", err)
	}
	if len(h.get("stale")) != 0 {
		t.Fatal("old terminal snapshot replaced live lifecycle")
	}
}

func TestHistoryLegacyMigrationPreservesRecordsAndExportContract(t *testing.T) {
	file := filepath.Join(ownedTestDir(t), "history.json")
	legacy := []byte(`{"legacy":{"gid":"legacy","status":"complete","recordedAt":123,"files":[{"path":"original.bin","length":"7"}]}}`)
	if err := os.WriteFile(file, legacy, 0600); err != nil {
		t.Fatal(err)
	}
	h, err := openHistory(file)
	if err != nil {
		t.Fatal(err)
	}
	unchanged, err := os.ReadFile(file)
	if err != nil || string(unchanged) != string(legacy) {
		t.Fatal("opening rewrote legacy history", err)
	}
	if err = h.forget("other"); err != nil {
		t.Fatal(err)
	}
	reopened, err := openHistory(file)
	if err != nil {
		t.Fatal(err)
	}
	if historyRecordedAt(reopened.get("legacy")) != 123 || !reopened.forgotten["other"] {
		t.Fatal("migration lost records or hidden IDs")
	}
	exported, count, err := reopened.export(false)
	if err != nil {
		t.Fatal(err)
	}
	var value struct {
		Version int               `json:"version"`
		Records []json.RawMessage `json:"records"`
	}
	if err = json.Unmarshal(exported, &value); err != nil || value.Version != 1 || len(value.Records) != 1 || count != 1 {
		t.Fatal("history export compatibility changed", err, string(exported))
	}
}

func TestHistorySuppressionReleaseRollbackAndRestart(t *testing.T) {
	file := filepath.Join(ownedTestDir(t), "history.json")
	h, err := openHistory(file)
	if err != nil {
		t.Fatal(err)
	}
	if err = h.forget("reused"); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(ownedTestDir(t), "blocking-file")
	if err = os.WriteFile(blocker, []byte("owned failure fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	h.path = filepath.Join(blocker, "history.json")
	live := []json.RawMessage{json.RawMessage(`{"gid":"reused","status":"active"}`)}
	got, err := h.merge(live)
	if err == nil || len(got) != 1 || !h.forgotten["reused"] {
		t.Fatal("failed save hid active row or changed suppression", err)
	}
	h, err = openHistory(file)
	if err != nil || !h.forgotten["reused"] {
		t.Fatal("failed release changed saved suppression", err)
	}
	if _, err = h.merge(live); err != nil {
		t.Fatal(err)
	}
	h, err = openHistory(file)
	if err != nil || h.forgotten["reused"] {
		t.Fatal("active lifecycle release did not persist", err)
	}
	got, err = h.merge([]json.RawMessage{json.RawMessage(`{"gid":"reused","status":"complete"}`)})
	if err != nil || len(got) != 1 {
		t.Fatal("new completion stayed hidden", err)
	}
}

func TestHistoryRejectsDamagedVersionedStateWithoutChangingOriginal(t *testing.T) {
	for i, original := range []string{
		`null`, `{"version":3,"records":{},"forgotten":{}}`, `{"version":2,"records":null,"forgotten":{}}`,
		`{"version":2,"records":{},"forgotten":null}`, `{"version":2,"records":{}}`,
		`{"version":2,"records":{},"forgotten":{"a":false}}`,
		`{"version":2,"records":{"a":{"gid":"a","status":"complete"}},"forgotten":{"a":true}}`,
		`{"version":2,"records":{},"forgotten":{},"extra":1}`,
		"{\"a\":\"\xff\"}",
	} {
		file := filepath.Join(ownedTestDir(t), fmt.Sprintf("history-%d.json", i))
		if err := os.WriteFile(file, []byte(original), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := openHistory(file); err == nil {
			t.Fatalf("accepted damaged history %d", i)
		}
		kept, err := os.ReadFile(file)
		if err != nil || string(kept) != original {
			t.Fatal("damaged original changed", err)
		}
	}
}

func TestHistoryForgottenLimitRefusesWithoutDroppingPriorChoices(t *testing.T) {
	file := filepath.Join(ownedTestDir(t), "history.json")
	h, err := openHistory(file)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxForgottenHistory; i++ {
		h.forgotten[fmt.Sprintf("forgotten-%d", i)] = true
	}
	h.rows["keep"] = json.RawMessage(`{"gid":"keep","status":"complete"}`)
	if err = h.write(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err = h.forget("keep"); err == nil {
		t.Fatal("accepted suppression beyond limit")
	}
	if len(h.get("keep")) == 0 || h.forgotten["keep"] || len(h.forgotten) != maxForgottenHistory {
		t.Fatal("limit dropped an existing choice")
	}
	if err = h.forget("forgotten-0"); err != nil {
		t.Fatal("existing choice was not idempotent", err)
	}
	after, err := os.ReadFile(file)
	if err != nil || string(after) != string(before) {
		t.Fatal("limit or idempotent call changed saved choices", err)
	}
}

func TestHistoryActiveIDReuseReleasesForgottenResult(t *testing.T) {
	file := filepath.Join(ownedTestDir(t), "history.json")
	h, err := openHistory(file)
	if err != nil {
		t.Fatal(err)
	}
	if err = h.forget("reused"); err != nil {
		t.Fatal(err)
	}
	if _, err = h.merge([]json.RawMessage{json.RawMessage(`{"gid":"reused","status":"active"}`)}); err != nil {
		t.Fatal(err)
	}
	got, err := h.merge([]json.RawMessage{json.RawMessage(`{"gid":"reused","status":"complete"}`)})
	if err != nil || len(got) != 1 || len(h.get("reused")) == 0 {
		t.Fatal("forgotten old result hid the new lifecycle", len(got), err)
	}
}
