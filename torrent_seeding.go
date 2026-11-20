package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	rain "github.com/cenkalti/rain/v2/torrent"
)

func (n *NativeTorrents) seeding(gid string, minutes int, approved bool, write bool) (rain.SeedingState, error) {
	return n.seedingPolicy(gid, minutes, 0, approved, write)
}

func (n *NativeTorrents) seedingPolicy(gid string, minutes, ratioPercent int, approved bool, write bool) (rain.SeedingState, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return rain.SeedingState{}, errors.New("native torrent engine unavailable")
	}
	tor := n.session.GetTorrent(gid)
	if tor == nil {
		return rain.SeedingState{}, errors.New("native torrent not found")
	}
	if write {
		if minutes < 0 || minutes > 1440 {
			return rain.SeedingState{}, errors.New("choose 1 to 1440 minutes, or 0 to stop sharing")
		}
		if minutes > 0 && !approved {
			return rain.SeedingState{}, errors.New("approve sharing this torrent's files with peers first")
		}
		until := time.Time{}
		if minutes > 0 {
			until = time.Now().Add(time.Duration(minutes) * time.Minute)
		}
		if err := tor.SetSeedingPolicy(until, ratioPercent); err != nil {
			return rain.SeedingState{}, err
		}
	}
	return tor.SeedingState(), nil
}

func (a *App) torrentSeedingAPI(w http.ResponseWriter, r *http.Request) {
	if a.engine == nil || a.engine.Native == nil {
		http.Error(w, "native torrents unavailable", 503)
		return
	}
	gid := r.URL.Query().Get("gid")
	minutes := 0
	approved := false
	ratioPercent := 0
	if r.Method == http.MethodPost {
		var input struct {
			GID          string `json:"gid"`
			Minutes      *int   `json:"minutes"`
			Approved     bool   `json:"approved"`
			RatioPercent int    `json:"ratioPercent"`
		}
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
		d.DisallowUnknownFields()
		if d.Decode(&input) != nil || d.Decode(new(any)) != io.EOF || input.Minutes == nil {
			http.Error(w, "invalid seeding choice", 400)
			return
		}
		gid = input.GID
		minutes = *input.Minutes
		approved = input.Approved
		ratioPercent = input.RatioPercent
	} else if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", 405)
		return
	}
	state, err := a.engine.Native.seedingPolicy(gid, minutes, ratioPercent, approved, r.Method == http.MethodPost)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	respond(w, state)
}
