package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

type Appearance struct {
	Theme string `json:"theme"`
}

func (a Appearance) valid() bool {
	return a.Theme == "light" || a.Theme == "dark" || a.Theme == "system"
}

type Preferences struct {
	mu         sync.Mutex
	path       string
	appearance Appearance
}

func openPreferences(path string) (*Preferences, error) {
	p := &Preferences{path: path, appearance: Appearance{Theme: "light"}}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return p, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1024 {
		return nil, errors.New("appearance preferences require recovery; original file retained")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("appearance preferences unavailable")
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 1024))
	d.DisallowUnknownFields()
	if d.Decode(&p.appearance) != nil || d.Decode(new(any)) != io.EOF || !p.appearance.valid() {
		return nil, errors.New("invalid appearance preferences; original file retained")
	}
	return p, nil
}
func (p *Preferences) get() Appearance { p.mu.Lock(); defer p.mu.Unlock(); return p.appearance }
func (p *Preferences) set(a Appearance) error {
	if !a.valid() {
		return errors.New("choose light, dark or system appearance")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	body, _ := json.Marshal(a)
	f, err := os.CreateTemp(filepath.Dir(p.path), ".appearance-*")
	if err != nil {
		return errors.New("cannot save appearance preferences")
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(body)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil || os.Rename(f.Name(), p.path) != nil {
		return errors.New("cannot save appearance preferences")
	}
	p.appearance = a
	return nil
}

func (a *App) preferencesAPI(w http.ResponseWriter, r *http.Request) {
	if a.preferences == nil {
		http.Error(w, "Appearance preferences unavailable; other downloads still work", 503)
		return
	}
	switch r.Method {
	case http.MethodGet:
		respond(w, a.preferences.get())
	case http.MethodPost:
		var value Appearance
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
		d.DisallowUnknownFields()
		if d.Decode(&value) != nil || d.Decode(new(any)) != io.EOF || !value.valid() {
			http.Error(w, "invalid appearance preferences", 400)
			return
		}
		if err := a.preferences.set(value); err != nil {
			http.Error(w, err.Error(), 503)
			return
		}
		respond(w, a.preferences.get())
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", 405)
	}
}
