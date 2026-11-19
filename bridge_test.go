package main

import (
	"net/http/httptest"
	"testing"
)

func TestBrowserBridgeIsolation(t *testing.T) {
	b := &Bridge{}
	origin := "chrome-extension://aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	secret, ok := b.pair(origin)
	if !ok {
		t.Fatal("pair failed")
	}
	if _, ok = b.pair("https://evil.example"); ok {
		t.Fatal("paired website")
	}
	a := &App{host: "127.0.0.1:8123", bridge: b, token: "app-token"}
	for _, tc := range []struct {
		method, path, origin, token string
		want                        int
	}{{"OPTIONS", "/bridge/add", origin, "", 204}, {"GET", "/bridge/ping", origin, secret, 200}, {"POST", "/bridge/control", origin, secret, 403}, {"GET", "/bridge/ping", "https://evil.example", secret, 403}, {"GET", "/bridge/ping", origin, "app-token", 401}} {
		r := httptest.NewRequest(tc.method, "http://"+a.host+tc.path, nil)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("X-ODM-Extension-Origin", tc.origin)
		r.Header.Set("Authorization", "Bearer "+tc.token)
		w := httptest.NewRecorder()
		a.handler().ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s %s got %d want %d", tc.method, tc.path, w.Code, tc.want)
		}
	}
	b.revoke()
	r := httptest.NewRequest("GET", "http://"+a.host+"/bridge/ping", nil)
	r.Header.Set("Origin", origin)
	r.Header.Set("Authorization", "Bearer "+secret)
	w := httptest.NewRecorder()
	a.handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("revocation failed")
	}
}
