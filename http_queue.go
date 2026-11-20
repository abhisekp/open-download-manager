package main

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/abhisekp/open-download-manager/internal/transfers"
)

// Authentication, Origin and Host checks are applied by the shared handler.
func (a *App) httpQueueAPI(w http.ResponseWriter, r *http.Request) {
	if a.engine == nil || a.engine.HTTP == nil {
		http.Error(w, "native HTTP queue unavailable", http.StatusServiceUnavailable)
		return
	}
	m := a.engine.HTTP
	if r.Method == http.MethodGet {
		respond(w, map[string]any{"settings": m.QueueSettings(), "jobs": m.Jobs()})
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		Action        string                  `json:"action"`
		URL           string                  `json:"url"`
		StartAt       int64                   `json:"startAt"`
		SingleStream  bool                    `json:"singleStream"`
		CredentialRef string                  `json:"credentialRef"`
		Approved      bool                    `json:"approved"`
		RetryLimit    *int                    `json:"retryLimit"`
		GID           string                  `json:"gid"`
		Position      string                  `json:"position"`
		Settings      transfers.QueueSettings `json:"settings"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil || decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "invalid queue request", 400)
		return
	}
	var err error
	var gid string
	switch in.Action {
	case "add":
		if in.GID != "" || in.Position != "" || in.Approved || in.Settings != (transfers.QueueSettings{}) {
			http.Error(w, "unexpected add fields", 400)
			return
		}
		gid, err = m.AddWithOptions(in.URL, transfers.AddOptions{StartAt: in.StartAt, SingleStream: in.SingleStream, CredentialRef: in.CredentialRef, RetryLimit: in.RetryLimit})
	case "login":
		if in.RetryLimit != nil {
			http.Error(w, "unexpected login fields", 400)
			return
		}
		if !in.Approved || in.URL != "" || in.StartAt != 0 || in.SingleStream || in.Position != "" || in.Settings != (transfers.QueueSettings{}) {
			http.Error(w, "approve restarting this paused download with the selected login", 400)
			return
		}
		err = m.SetCredential(in.GID, in.CredentialRef)
	case "move":
		if in.RetryLimit != nil {
			http.Error(w, "unexpected move fields", 400)
			return
		}
		if in.URL != "" || in.StartAt != 0 || in.SingleStream || in.CredentialRef != "" || in.Approved || in.Settings != (transfers.QueueSettings{}) {
			http.Error(w, "unexpected move fields", 400)
			return
		}
		err = m.Move(in.GID, in.Position)
	case "settings":
		if in.RetryLimit != nil {
			http.Error(w, "unexpected settings fields", 400)
			return
		}
		if in.URL != "" || in.StartAt != 0 || in.SingleStream || in.CredentialRef != "" || in.Approved || in.GID != "" || in.Position != "" {
			http.Error(w, "unexpected settings fields", 400)
			return
		}
		err = m.SetQueue(in.Settings)
	default:
		http.Error(w, "invalid queue action", 400)
		return
	}
	if err != nil {
		http.Error(w, "Queue change rejected. Check the URL, future start time, pending transfer and queue limits; also check app data folder access.", 400)
		return
	}
	respond(w, map[string]any{"ok": true, "gid": gid})
}
