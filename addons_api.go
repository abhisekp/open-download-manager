package main

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/abhisekp/open-download-manager/internal/addons"
)

func (a *App) addonsAPI(w http.ResponseWriter, r *http.Request) {
	if a.addons == nil {
		http.Error(w, "add-on storage unavailable", 503)
		return
	}
	if r.URL.Path == "/api/addons" && r.Method == http.MethodGet {
		respond(w, a.addons.List())
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	switch r.URL.Path {
	case "/api/addons/inspect", "/api/addons/install":
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, addons.MaxPackage))
		if err != nil {
			http.Error(w, "package exceeds 8 MiB", 413)
			return
		}
		pkg, err := addons.Inspect(raw)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if r.URL.Path == "/api/addons/inspect" {
			respond(w, struct {
				addons.Info
				License string `json:"license"`
			}{pkg.Info, string(pkg.License)})
			return
		}
		info, err := a.addons.Install(raw, r.Header.Get("X-ODM-Approve-SHA256"))
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		respond(w, info)
		return
	}
	var in struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
		URL     string `json:"url"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	var err error
	switch r.URL.Path {
	case "/api/addons/enable":
		err = a.addons.SetEnabled(in.ID, in.Enabled)
	case "/api/addons/uninstall":
		err = a.addons.Uninstall(in.ID)
	case "/api/addons/resolve":
		var urls []string
		urls, err = a.addons.Resolve(r.Context(), in.ID, in.URL)
		if err == nil {
			respond(w, map[string]any{"urls": urls})
			return
		}
	default:
		http.Error(w, "not found", 404)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	respond(w, map[string]bool{"ok": true})
}
