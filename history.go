package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// Bounded local history stores terminal engine records; unfinished jobs remain in aria2's session.
type History struct {
	mu        sync.Mutex
	path      string
	rows      map[string]json.RawMessage
	forgotten map[string]bool
}

func openHistory(path string) (*History, error) {
	h := &History{path: path, rows: map[string]json.RawMessage{}, forgotten: map[string]bool{}}
	b, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return h, nil
	}
	if e != nil {
		return nil, e
	}
	if len(b) > 16<<20 {
		return nil, errors.New("history exceeds supported size")
	}
	if e = json.Unmarshal(b, &h.rows); e != nil {
		return nil, errors.New("history is corrupt; preserve it before recovery")
	}
	if h.rows == nil {
		h.rows = map[string]json.RawMessage{}
	}
	for gid, raw := range h.rows {
		var row struct{ GID, Status string }
		if json.Unmarshal(raw, &row) != nil || row.GID != gid || gid == "" || (row.Status != "complete" && row.Status != "error" && row.Status != "removed") {
			return nil, errors.New("invalid history record; preserve history before recovery")
		}
	}
	return h, nil
}
func (h *History) write() error {
	keys := make([]string, 0, len(h.rows))
	for k := range h.rows {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for len(keys) > 500 {
		delete(h.rows, keys[0])
		keys = keys[1:]
	}
	b, e := json.Marshal(h.rows)
	if e != nil {
		return e
	}
	if len(b) > 16<<20 {
		return errors.New("history exceeds supported size")
	}
	if e = os.MkdirAll(filepath.Dir(h.path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(h.path), "history-*.tmp")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(tmp, h.path)
}
func (h *History) merge(current []json.RawMessage) ([]json.RawMessage, error) {
	if h == nil {
		return current, nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	seen := map[string]bool{}
	changed := false
	filtered := make([]json.RawMessage, 0, len(current))
	for _, raw := range current {
		var row struct{ GID, Status string }
		if json.Unmarshal(raw, &row) != nil {
			continue
		}
		if h.forgotten[row.GID] {
			continue
		}
		filtered = append(filtered, raw)
		seen[row.GID] = true
		if row.Status == "complete" || row.Status == "error" || row.Status == "removed" {
			if !bytes.Equal(h.rows[row.GID], raw) {
				h.rows[row.GID] = append(json.RawMessage(nil), raw...)
				changed = true
			}
		}
	}
	current = filtered
	keys := make([]string, 0, len(h.rows))
	for k := range h.rows {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, gid := range keys {
		if !seen[gid] {
			var row map[string]any
			_ = json.Unmarshal(h.rows[gid], &row)
			row["archived"] = true
			raw, _ := json.Marshal(row)
			current = append(current, raw)
		}
	}
	if changed {
		if e := h.write(); e != nil {
			return nil, e
		}
	}
	return current, nil
}
func (h *History) get(gid string) json.RawMessage {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return append(json.RawMessage(nil), h.rows[gid]...)
}
func (h *History) forget(gid string) error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.rows, gid)
	h.forgotten[gid] = true
	return h.write()
}
