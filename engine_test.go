package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestURIValidation(t *testing.T) {
	for _, u := range []string{"https://example.org/file", "http://127.0.0.1:9000/test", "magnet:?xt=urn:btih:abc"} {
		if e := validateURI(u); e != nil {
			t.Errorf("%s: %v", u, e)
		}
	}
	for _, u := range []string{"file:///etc/passwd", "javascript:alert(1)", "https://user:pass@example.com", "magnet:?dn=bad", "https://"} {
		if validateURI(u) == nil {
			t.Errorf("accepted %s", u)
		}
	}
}
func TestRPC(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Method string
			Params []string
		}
		json.NewDecoder(r.Body).Decode(&q)
		if q.Method != "aria2.getVersion" || q.Params[0] != "token:test" {
			t.Error("wrong RPC authentication")
		}
		w.Write([]byte(`{"result":{"version":"1"}}`))
	}))
	defer s.Close()
	e := newEngine(s.URL, "test")
	raw, err := e.Call(context.Background(), "getVersion")
	if err != nil || !strings.Contains(string(raw), "version") {
		t.Fatal(err)
	}
}
func TestRPCError(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"error":{"code":1,"message":"not found"}}`))
	}))
	defer s.Close()
	if _, e := newEngine(s.URL, "").Call(context.Background(), "tellStatus"); e == nil {
		t.Fatal("expected RPC error")
	}
}
func TestSecurityBoundary(t *testing.T) {
	a := &App{token: "test", host: "127.0.0.1:8123"}
	for _, tc := range []struct {
		host, origin, auth string
		want               int
	}{{"evil.test", "", "Bearer test", 403}, {a.host, "https://evil.test", "Bearer test", 403}, {a.host, "", "", 401}, {a.host, "", "Bearer test", 200}} {
		r := httptest.NewRequest("GET", "http://"+tc.host+"/api/extensions", nil)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Authorization", tc.auth)
		w := httptest.NewRecorder()
		a.handler().ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("got %d want %d", w.Code, tc.want)
		}
	}
}
func TestTrackerParser(t *testing.T) {
	rows, e := parseTrackers(strings.NewReader("udp://tracker.example:80/announce\n\nhttps://tracker.example/announce\nudp://tracker.example:80/announce\n"))
	if e != nil || len(rows) != 2 {
		t.Fatal(rows, e)
	}
	for _, input := range []string{"", "<html>offline</html>", "file:///bad", "https://user:pass@example.com/announce", strings.Repeat("x", 70000)} {
		if _, e := parseTrackers(strings.NewReader(input)); e == nil {
			t.Fatal("accepted invalid tracker feed")
		}
	}
}
func TestExtensionPermission(t *testing.T) {
	m := trackerManifest
	m.Endpoint = "https://evil.example"
	if validateManifest(m) == nil {
		t.Fatal("accepted arbitrary endpoint")
	}
	a := &App{token: "test", host: "127.0.0.1:8123"}
	r := httptest.NewRequest("POST", "http://"+a.host+"/api/trackers", strings.NewReader(`{"enabled":false}`))
	r.Header.Set("Authorization", "Bearer test")
	w := httptest.NewRecorder()
	a.handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
