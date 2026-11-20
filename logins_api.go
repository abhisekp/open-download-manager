package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/abhisekp/open-download-manager/internal/credentials"
)

func (a *App) loginsAPI(w http.ResponseWriter, r *http.Request) {
	if a.logins == nil {
		http.Error(w, "Saved logins unavailable; downloads without a login still work", 503)
		return
	}
	if r.Method == http.MethodGet {
		respond(w, map[string]any{"logins": a.logins.List(), "busy": a.logins.Busy()})
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", 405)
		return
	}
	var in struct {
		Action   string `json:"action"`
		ID       string `json:"id"`
		Label    string `json:"label"`
		Origin   string `json:"origin"`
		Username string `json:"username"`
		Password string `json:"password"`
		Approved bool   `json:"approved"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil || d.Decode(new(any)) != io.EOF {
		http.Error(w, "invalid saved-login request", 400)
		return
	}
	var err error
	var entry credentials.Entry
	switch in.Action {
	case "save":
		if !in.Approved {
			http.Error(w, "approve storing this login for the shown HTTPS website first", 403)
			return
		}
		if in.ID != "" {
			http.Error(w, "save creates a new login; it does not overwrite an existing key", 400)
			return
		}
		entry, err = a.logins.Create(in.Label, in.Origin, in.Username, in.Password)
	case "remove", "check":
		if in.Label != "" || in.Origin != "" || in.Username != "" || in.Password != "" {
			http.Error(w, "unexpected login action fields", 400)
			return
		}
		if in.Action == "remove" && !in.Approved {
			http.Error(w, "approve removing this saved login first", 403)
			return
		}
		err = a.logins.Act(in.ID, in.Action)
	default:
		http.Error(w, "invalid saved-login action", 400)
		return
	}
	if err != nil {
		code := 400
		if errors.Is(err, credentials.ErrBusy) {
			code = 409
		}
		if errors.Is(err, credentials.ErrStorage) || errors.Is(err, credentials.ErrUnavailable) {
			code = 503
		}
		http.Error(w, err.Error(), code)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	respond(w, map[string]any{"accepted": true, "login": entry})
}
