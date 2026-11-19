package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abhisekp/open-download-manager/internal/addons"
)

func TestAddonAPIRequiresUIIdentity(t *testing.T) {
	store, err := addons.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := &App{host: "127.0.0.1:8431", token: "ui-secret", addons: store}
	handler := app.handler()
	for _, test := range []struct {
		token, origin string
		status        int
	}{{"", "", 401}, {"browser-pairing-token", "", 401}, {"ui-secret", "https://untrusted.example", 403}, {"ui-secret", "http://127.0.0.1:8431", 200}} {
		r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8431/api/addons", nil)
		r.Header.Set("Authorization", "Bearer "+test.token)
		r.Header.Set("Origin", test.origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != test.status {
			t.Fatalf("identity boundary: expected %d got %d", test.status, w.Code)
		}
	}
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8431/api/addons/enable", strings.NewReader(`{"id":"missing","enabled":true}`))
	r.Header.Set("Authorization", "Bearer ui-secret")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatalf("unknown add-on mutation accepted: %d", w.Code)
	}
}
