package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppearancePersistenceAndOwnerBoundary(t *testing.T) {
	file := filepath.Join(t.TempDir(), "appearance.json")
	prefs, err := openPreferences(file)
	if err != nil {
		t.Fatal(err)
	}
	if prefs.get().Theme != "light" {
		t.Fatal("approved light default changed")
	}
	app := &App{preferences: prefs, host: "127.0.0.1:9911", token: "appearance-owner"}
	for _, tc := range []struct {
		body, token, origin string
		code                int
	}{
		{`{"theme":"dark"}`, "", "", 401},
		{`{"theme":"dark"}`, "browser-token", "", 401},
		{`{"theme":"dark"}`, app.token, "https://other.invalid", 403},
		{`{"theme":"invalid"}`, app.token, "", 400},
		{`{"theme":"dark","path":"other-file"}`, app.token, "", 400},
		{`{"theme":"dark"} {}`, app.token, "", 400},
		{`{"theme":"dark"}`, app.token, "http://" + app.host, 200},
	} {
		req := httptest.NewRequest("POST", "http://"+app.host+"/api/preferences", strings.NewReader(tc.body))
		req.Header.Set("Authorization", "Bearer "+tc.token)
		req.Header.Set("Origin", tc.origin)
		out := httptest.NewRecorder()
		app.handler().ServeHTTP(out, req)
		if out.Code != tc.code {
			t.Fatalf("preferences status %d want %d", out.Code, tc.code)
		}
	}
	reopened, err := openPreferences(file)
	if err != nil || reopened.get().Theme != "dark" {
		t.Fatal("appearance was not durable", err)
	}
	before := prefs.get()
	prefs.path = filepath.Join(t.TempDir(), "missing", "appearance.json")
	if err := prefs.set(Appearance{Theme: "system"}); err == nil || prefs.get() != before {
		t.Fatal("failed appearance write changed current preference")
	}
	invalid := []byte(`{"theme":"unexpected"}`)
	if err := os.WriteFile(file, invalid, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := openPreferences(file); err == nil {
		t.Fatal("corrupt preference accepted")
	}
	actual, _ := os.ReadFile(file)
	if string(actual) != string(invalid) {
		t.Fatal("corrupt preference overwritten")
	}
}
