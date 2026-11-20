package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abhisekp/open-download-manager/internal/transfers"
)

func TestHTTPQueueCapabilityAndStrictInput(t *testing.T) {
	m, err := transfers.Open(transfers.Config{DataDir: filepath.Join(t.TempDir(), "data"), Downloads: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	a := &App{engine: &Engine{HTTP: m}, token: "test-ui-capability", host: "127.0.0.1:9876"}
	handler := a.handler()
	for _, tc := range []struct {
		body, auth, origin string
		code               int
	}{
		{`{"action":"settings","settings":{"concurrent":1,"perHost":1}}`, "", "", 401},
		{`{"action":"settings","settings":{"concurrent":1,"perHost":1}}`, "browser-submission-token", "", 401},
		{`{"action":"settings","settings":{"concurrent":1,"perHost":1}}`, a.token, "https://untrusted.invalid", 403},
		{`{"action":"settings","settings":{"concurrent":1,"perHost":1},"hidden":true}`, a.token, "", 400},
		{`{"action":"settings","settings":{"concurrent":1,"perHost":1}} {}`, a.token, "", 400},
		{`{"action":"settings","settings":{"concurrent":1,"perHost":3}}`, a.token, "", 400},
		{`{"action":"settings","singleStream":true,"settings":{"concurrent":1,"perHost":1}}`, a.token, "", 400},
		{`{"action":"add","url":"http://user:secret@example.invalid/file"}`, a.token, "", 400},
		{`{"action":"add","url":"file:///private"}`, a.token, "", 400},
		{`{"action":"settings","url":"http://example.invalid","settings":{"concurrent":1,"perHost":1}}`, a.token, "", 400},
		{`{"action":"settings","settings":{"concurrent":1,"perHost":1}}`, a.token, "http://" + a.host, 200},
	} {
		r := httptest.NewRequest("POST", "http://"+a.host+"/api/http-queue", strings.NewReader(tc.body))
		if tc.auth != "" {
			r.Header.Set("Authorization", "Bearer "+tc.auth)
		}
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.code {
			t.Fatalf("queue status %d, wanted %d", w.Code, tc.code)
		}
	}
	if len(m.Jobs()) != 0 {
		t.Fatal("invalid input caused downloads")
	}
	if m.QueueSettings() != (transfers.QueueSettings{Concurrent: 1, PerHost: 1}) {
		t.Fatal("authorized settings change not applied")
	}
	r := httptest.NewRequest(http.MethodGet, "http://"+a.host+"/api/http-queue", nil)
	r.Header.Set("Authorization", "Bearer "+a.token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"concurrent":1`) {
		t.Fatal("cannot read queue settings")
	}
	id, err := m.AddPlanned("https://example.invalid/later", time.Now().Add(time.Hour).UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRequest(http.MethodPost, "http://"+a.host+"/api/http-queue", strings.NewReader(`{"action":"move","gid":"`+id+`","position":"first"}`))
	r.Header.Set("Authorization", "Bearer "+a.token)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal("authorized reorder failed")
	}
}
