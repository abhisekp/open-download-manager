package main

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"regexp"
	"sync"
	"time"
)

type bridgeGrant struct {
	Token  string
	Window time.Time
	Count  int
}
type Bridge struct {
	mu     sync.Mutex
	grants map[string]*bridgeGrant
}

var extensionOrigin = regexp.MustCompile(`^(chrome-extension://[a-p]{32}|moz-extension://[a-fA-F0-9-]{36}|safari-web-extension://[a-zA-Z0-9.-]{1,128})$`)

func (b *Bridge) pair(origin string) (string, bool) {
	if !extensionOrigin.MatchString(origin) {
		return "", false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.grants == nil {
		b.grants = map[string]*bridgeGrant{}
	}
	secret := token()
	b.grants[origin] = &bridgeGrant{Token: secret}
	return secret, true
}
func (b *Bridge) revoke() { b.mu.Lock(); defer b.mu.Unlock(); b.grants = map[string]*bridgeGrant{} }
func (a *App) bridgeAPI(w http.ResponseWriter, r *http.Request) {
	identity := r.Header.Get("X-ODM-Extension-Origin")
	origin := r.Header.Get("Origin")
	if identity == "" {
		identity = origin
	}
	if a.bridge == nil || !extensionOrigin.MatchString(identity) || (origin != "" && origin != identity) {
		http.Error(w, "unpaired browser origin", 403)
		return
	}
	a.bridge.mu.Lock()
	grant := a.bridge.grants[identity]
	if grant == nil {
		a.bridge.mu.Unlock()
		http.Error(w, "browser is not paired", 403)
		return
	}
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
	}
	if r.Method == "OPTIONS" {
		a.bridge.mu.Unlock()
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-ODM-Extension-Origin")
		w.WriteHeader(204)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+grant.Token)) != 1 {
		a.bridge.mu.Unlock()
		http.Error(w, "invalid browser token", 401)
		return
	}
	if time.Since(grant.Window) > time.Minute {
		grant.Window = time.Now()
		grant.Count = 0
	}
	grant.Count++
	over := grant.Count > 30
	a.bridge.mu.Unlock()
	if over {
		http.Error(w, "browser rate limit reached", 429)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/bridge/ping" {
		respond(w, map[string]any{"name": "Open Download Manager", "version": version, "scope": "downloads:create"})
		return
	}
	if r.Method != "POST" || r.URL.Path != "/bridge/add" {
		http.Error(w, "browser permission only allows adding links", 403)
		return
	}
	var in struct {
		URL string `json:"url"`
	}
	if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&in); e != nil {
		http.Error(w, "invalid link request", 400)
		return
	}
	if e := validateURI(in.URL); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	gid, e := a.engine.Call(r.Context(), "addUri", []string{in.URL})
	if e != nil {
		http.Error(w, "transfer engine rejected the link", 502)
		return
	}
	_, _ = a.engine.Call(r.Context(), "saveSession")
	respond(w, map[string]any{"gid": gid})
}
