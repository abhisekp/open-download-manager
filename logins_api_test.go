package main

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abhisekp/open-download-manager/internal/credentials"
)

type loginFixtureBackend struct {
	mu     sync.Mutex
	values map[string]string
}

func (b *loginFixtureBackend) Set(service, id, value string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.values[service+id] = value
	return nil
}
func (b *loginFixtureBackend) Get(service, id string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	v, ok := b.values[service+id]
	if !ok {
		return "", credentials.ErrMissing
	}
	return v, nil
}
func (b *loginFixtureBackend) Delete(service, id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.values, service+id)
	return nil
}
func (*loginFixtureBackend) Missing(err error) bool { return errors.Is(err, credentials.ErrMissing) }

func TestSavedLoginAPIIsOwnerOnlyAndNeverReturnsSecrets(t *testing.T) {
	backend := &loginFixtureBackend{values: map[string]string{}}
	store, err := credentials.Open(filepath.Join(t.TempDir(), "references.json"), "owned-api-fixture", backend)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	a := &App{logins: store, token: "owner-ui-key", host: "127.0.0.1:9898"}
	handler := a.handler()
	call := func(body, auth, origin string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("POST", "http://"+a.host+"/api/logins", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+auth)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	valid := `{"action":"save","label":"QA login","origin":"https://example.test","username":"fixture-account","password":"fixture-secret","approved":true}`
	for _, tc := range []struct {
		body, auth, origin string
		code               int
	}{
		{valid, "", "", 401}, {valid, "browser-only-key", "", 401}, {valid, a.token, "https://evil.test", 403},
		{strings.Replace(valid, `"approved":true`, `"approved":false`, 1), a.token, "", 403},
		{strings.TrimSuffix(valid, "}") + `,"unknown":true}`, a.token, "", 400},
		{valid + ` {}`, a.token, "", 400},
	} {
		if w := call(tc.body, tc.auth, tc.origin); w.Code != tc.code {
			t.Fatal("login boundary status", w.Code, tc.code)
		}
	}
	if len(store.List()) != 0 {
		t.Fatal("rejected login request had side effects")
	}
	w := call(valid, a.token, "http://"+a.host)
	if w.Code != 202 || !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
		t.Fatal("save not accepted as JSON")
	}
	if strings.Contains(w.Body.String(), "fixture-account") || strings.Contains(w.Body.String(), "fixture-secret") {
		t.Fatal("save response returned secret fields")
	}
	deadline := time.Now().Add(3 * time.Second)
	for store.Busy() {
		if time.Now().After(deadline) {
			t.Fatal("save stuck")
		}
		time.Sleep(time.Millisecond)
	}
	entry := store.List()[0]
	r := httptest.NewRequest("GET", "http://"+a.host+"/api/logins", nil)
	r.Header.Set("Authorization", "Bearer "+a.token)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	var listed struct {
		Logins []credentials.Entry `json:"logins"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &listed) != nil || len(listed.Logins) != 1 || listed.Logins[0].State != "ready" {
		t.Fatal("cannot list ready reference")
	}
	if strings.Contains(w.Body.String(), "fixture-account") || strings.Contains(w.Body.String(), "fixture-secret") {
		t.Fatal("listing returned secret fields")
	}
	if w = call(`{"action":"remove","id":"`+entry.ID+`"}`, a.token, ""); w.Code != 403 {
		t.Fatal("removal without approval accepted")
	}
	if w = call(`{"action":"remove","id":"`+entry.ID+`","approved":true}`, a.token, ""); w.Code != 202 {
		t.Fatal("approved removal failed")
	}
	deadline = time.Now().Add(3 * time.Second)
	for store.Busy() {
		if time.Now().After(deadline) {
			t.Fatal("delete stuck")
		}
		time.Sleep(time.Millisecond)
	}
	if len(store.List()) != 0 {
		t.Fatal("saved login reference remained after successful removal")
	}
}
