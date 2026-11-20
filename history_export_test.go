package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abhisekp/open-download-manager/internal/transfers"
)

func TestHistoryWritesRollbackAndKeepLiveStatusVisible(t *testing.T) {
	file := filepath.Join(ownedTestDir(t), "history.json")
	h, err := openHistory(file)
	if err != nil {
		t.Fatal(err)
	}
	original := json.RawMessage(`{"gid":"a","status":"complete","files":[{"path":"original.bin"}]}`)
	if _, err := h.merge([]json.RawMessage{original}); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(ownedTestDir(t), "blocking-file")
	if err := os.WriteFile(blocker, []byte("owned storage failure fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	h.path = filepath.Join(blocker, "history.json")
	current := []json.RawMessage{original, json.RawMessage(`{"gid":"b","status":"error"}`)}
	live, err := h.merge(current)
	if err == nil || len(live) != 2 || len(h.get("b")) != 0 || len(h.get("a")) == 0 {
		t.Fatal("failed merge changed saved history or hid live transfers", err)
	}
	if err := h.forget("a"); err == nil || len(h.get("a")) == 0 || h.forgotten["a"] {
		t.Fatal("failed forget did not roll back")
	}
	h.path = file
	reopened, err := openHistory(file)
	if err != nil || len(reopened.get("a")) == 0 || len(reopened.get("b")) > 0 {
		t.Fatal("failed write changed durable history", err)
	}
}

func TestHistoryBoundUsesRecordedTimeRatherThanRandomIdentifier(t *testing.T) {
	h, err := openHistory(filepath.Join(ownedTestDir(t), "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 501; i++ {
		id := fmt.Sprintf("%04d", 500-i)
		h.rows[id] = json.RawMessage(fmt.Sprintf(`{"gid":%q,"status":"complete","recordedAt":%d}`, id, i+1))
	}
	if err := h.write(); err != nil {
		t.Fatal(err)
	}
	if len(h.rows) != 500 || len(h.get("0500")) != 0 || len(h.get("0000")) == 0 {
		t.Fatal("history cap discarded newer records instead of oldest observed record")
	}
}

func TestStaleHistoryCannotForgetPendingTransferOrHideLiveRows(t *testing.T) {
	h, err := openHistory(filepath.Join(ownedTestDir(t), "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := transfers.Open(transfers.Config{DataDir: filepath.Join(ownedTestDir(t), "data"), Downloads: ownedTestDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	id, err := m.AddPlanned("https://example.invalid/no-request", time.Now().Add(time.Hour).UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.merge([]json.RawMessage{json.RawMessage(fmt.Sprintf(`{"gid":%q,"status":"error"}`, id))}); err != nil {
		t.Fatal(err)
	}
	app := &App{engine: &Engine{HTTP: m}, history: h, host: "127.0.0.1:9933", token: "owned-forget-fixture"}
	req := httptest.NewRequest("POST", "http://"+app.host+"/api/control", strings.NewReader(fmt.Sprintf(`{"gid":%q,"action":"forget"}`, id)))
	req.Header.Set("Authorization", "Bearer "+app.token)
	out := httptest.NewRecorder()
	app.handler().ServeHTTP(out, req)
	if out.Code != 400 || !m.Has(id) || len(h.get(id)) == 0 || h.forgotten[id] {
		t.Fatal("stale terminal history hid or removed a pending transfer")
	}
	// Even an unavailable optional engine cannot make a later live row disappear
	// merely because the user previously forgot a stored result with that ID.
	h.forgotten[id] = true
	live, err := h.merge([]json.RawMessage{json.RawMessage(fmt.Sprintf(`{"gid":%q,"status":"active"}`, id))})
	if err != nil || len(live) != 1 {
		t.Fatal("history tombstone hid active progress", err)
	}
}

func TestLocalHistoryExportRedactionConsentAndDestination(t *testing.T) {
	h, err := openHistory(filepath.Join(ownedTestDir(t), "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(`{"gid":"owned-record","status":"complete","files":[{"path":"C:\\Users\\PRIVATE-PERSON\\file.bin","length":"42","uris":[{"uri":"https://fixture.invalid/file?token=PRIVATE-TOKEN"}]}]}`)
	if _, err := h.merge([]json.RawMessage{raw}); err != nil {
		t.Fatal(err)
	}
	downloads := ownedTestDir(t)
	keep := filepath.Join(downloads, "existing-user-file.json")
	if err := os.WriteFile(keep, []byte("retain this user file"), 0600); err != nil {
		t.Fatal(err)
	}
	app := &App{history: h, downloads: downloads, token: "history-owner", host: "127.0.0.1:9922"}
	call := func(body, token, origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "http://"+app.host+"/api/history-export", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Origin", origin)
		out := httptest.NewRecorder()
		app.handler().ServeHTTP(out, req)
		return out
	}
	for _, tc := range []struct {
		body, token, origin string
		code                int
	}{
		{`{"approved":true}`, "", "", 401},
		{`{"approved":true}`, "browser-token", "", 401},
		{`{"approved":true}`, app.token, "https://other.invalid", 403},
		{`{}`, app.token, "", 403},
		{`{"approved":true,"path":"../outside"}`, app.token, "", 400},
		{`{"approved":true} {}`, app.token, "", 400},
	} {
		if out := call(tc.body, tc.token, tc.origin); out.Code != tc.code {
			t.Fatalf("export boundary status %d want %d", out.Code, tc.code)
		}
	}
	initial, _ := os.ReadDir(downloads)
	if len(initial) != 1 {
		t.Fatal("rejected request wrote an export")
	}
	for _, sensitive := range []bool{false, true} {
		out := call(fmt.Sprintf(`{"approved":true,"includeSensitiveDetails":%t}`, sensitive), app.token, "http://"+app.host)
		var result struct {
			Name    string
			Records int
		}
		if out.Code != 200 || json.Unmarshal(out.Body.Bytes(), &result) != nil || filepath.Base(result.Name) != result.Name || result.Records != 1 {
			t.Fatal("approved export failed", out.Code)
		}
		data, err := os.ReadFile(filepath.Join(downloads, result.Name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "PRIVATE-TOKEN") != sensitive || strings.Contains(string(data), "PRIVATE-PERSON") != sensitive || !strings.Contains(string(data), "file.bin") {
			t.Fatal("export redaction choice ignored")
		}
	}
	actual, _ := os.ReadFile(keep)
	if string(actual) != "retain this user file" {
		t.Fatal("export overwrote user file")
	}
	app.historyExportMu.Lock()
	blocked := call(`{"approved":true}`, app.token, "")
	app.historyExportMu.Unlock()
	if blocked.Code != 409 {
		t.Fatal("concurrent export not bounded")
	}
}

func TestExportRefreshesJustCompletedNativeTransfer(t *testing.T) {
	h, err := openHistory(filepath.Join(ownedTestDir(t), "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	downloads := ownedTestDir(t)
	m, err := transfers.Open(transfers.Config{DataDir: filepath.Join(ownedTestDir(t), "data"), Downloads: downloads})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("owned export refresh fixture")) }))
	defer server.Close()
	id, err := m.Add(server.URL + "/fresh.bin")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for m.Jobs()[0].Status != "complete" {
		if time.Now().After(deadline) {
			t.Fatal("fixture did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(h.get(id)) != 0 {
		t.Fatal("fixture history was unexpectedly pre-flushed")
	}
	app := &App{engine: &Engine{HTTP: m, HTTPDirectory: downloads}, history: h, downloads: downloads, token: "owned-refresh", host: "127.0.0.1:9934"}
	req := httptest.NewRequest("POST", "http://"+app.host+"/api/history-export", strings.NewReader(`{"approved":true}`))
	req.Header.Set("Authorization", "Bearer "+app.token)
	out := httptest.NewRecorder()
	app.handler().ServeHTTP(out, req)
	var result struct{ Records int }
	if out.Code != 200 || json.Unmarshal(out.Body.Bytes(), &result) != nil || result.Records != 1 || len(h.get(id)) == 0 {
		t.Fatal("export omitted a just-completed transfer", out.Code, out.Body.String())
	}
}
