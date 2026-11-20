package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// Bounded local history stores terminal engine records; unfinished jobs remain in aria2's session.
type History struct {
	mu         sync.Mutex
	path       string
	rows       map[string]json.RawMessage
	forgotten  map[string]bool
	generation uint64
}

var errHistoryObservationStale = errors.New("history changed while reading engine results")

const maxForgottenHistory = 10000

type historyState struct {
	Version   int                        `json:"version"`
	Rows      map[string]json.RawMessage `json:"records"`
	Forgotten map[string]bool            `json:"forgotten"`
}

func validHistoryID(id string) bool {
	return id != "" && len(id) <= 128 && utf8.ValidString(id) && strings.IndexFunc(id, unicode.IsControl) < 0
}

func openHistory(path string) (*History, error) {
	h := &History{path: path, rows: map[string]json.RawMessage{}, forgotten: map[string]bool{}}
	info, e := os.Lstat(path)
	if os.IsNotExist(e) {
		return h, nil
	}
	if e != nil || !info.Mode().IsRegular() || info.Size() > 16<<20 {
		return nil, errors.New("history is not a supported regular file; original retained")
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	opened, e := f.Stat()
	if e != nil || !os.SameFile(info, opened) {
		return nil, errors.New("history file changed while opening; original retained")
	}
	b, e := io.ReadAll(io.LimitReader(f, (16<<20)+1))
	if e != nil {
		return nil, e
	}
	if len(b) > 16<<20 {
		return nil, errors.New("history exceeds supported size")
	}
	var shape map[string]json.RawMessage
	if !utf8.Valid(b) || json.Unmarshal(b, &shape) != nil || shape == nil {
		return nil, errors.New("history is corrupt; preserve it before recovery")
	}
	// Read the original row-map format; migrate only on the next successful
	// atomic write. An object-valued legacy GID named "version" remains valid.
	if raw, exists := shape["version"]; exists && !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
		var state historyState
		d := json.NewDecoder(bytes.NewReader(b))
		d.DisallowUnknownFields()
		if d.Decode(&state) != nil || state.Version != 2 || state.Rows == nil || state.Forgotten == nil {
			return nil, errors.New("unsupported history state; original retained")
		}
		h.rows = state.Rows
		if state.Forgotten != nil {
			h.forgotten = state.Forgotten
		}
	} else {
		h.rows = shape
	}
	if len(h.rows) > 500 || len(h.forgotten) > maxForgottenHistory {
		return nil, errors.New("history record limit exceeded; original retained")
	}
	for gid, hidden := range h.forgotten {
		if !validHistoryID(gid) || !hidden || h.rows[gid] != nil {
			return nil, errors.New("invalid hidden-history record; original retained")
		}
	}
	for gid, raw := range h.rows {
		var row struct{ GID, Status string }
		if json.Unmarshal(raw, &row) != nil || row.GID != gid || !validHistoryID(gid) || !terminalHistoryStatus(row.Status) {
			return nil, errors.New("invalid history record; preserve history before recovery")
		}
		if at := historyRecordedAt(raw); at < 0 || at > 253402300799000 {
			return nil, errors.New("invalid history timestamp; original retained")
		}
	}
	return h, nil
}
func (h *History) write() error {
	if len(h.forgotten) > maxForgottenHistory {
		return errors.New("hidden-history limit reached; saved records kept")
	}
	keys := make([]string, 0, len(h.rows))
	for k := range h.rows {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := historyRecordedAt(h.rows[keys[i]]), historyRecordedAt(h.rows[keys[j]])
		return a < b || a == b && keys[i] < keys[j]
	})
	for len(keys) > 500 {
		delete(h.rows, keys[0])
		keys = keys[1:]
	}
	b, e := json.Marshal(historyState{Version: 2, Rows: h.rows, Forgotten: h.forgotten})
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
// Capture before requesting engine results, so delayed replies cannot undo a
// subsequent history mutation. The token is process-local; ownership excludes
// another ODM process from writing the same data directory.
func (h *History) observation() uint64 {
	if h == nil {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.generation
}

// merge is for current local values. Asynchronous engine reads must instead
// capture observation before the read and pass it to mergeObserved.
func (h *History) merge(current []json.RawMessage) ([]json.RawMessage, error) {
	return h.mergeObserved(current, h.observation())
}

func (h *History) mergeObserved(current []json.RawMessage, observation uint64) ([]json.RawMessage, error) {
	if h == nil {
		return current, nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	before := maps.Clone(h.rows)
	beforeForgotten := maps.Clone(h.forgotten)
	fresh := observation == h.generation
	seen := map[string]bool{}
	changed := false
	filtered := make([]json.RawMessage, 0, len(current))
	for _, raw := range current {
		var row struct{ GID, Status string }
		if json.Unmarshal(raw, &row) != nil || !validHistoryID(row.GID) {
			continue
		}
		if h.forgotten[row.GID] && terminalHistoryStatus(row.Status) {
			continue
		}
		filtered = append(filtered, raw)
		seen[row.GID] = true
		if !fresh {
			continue
		}
		if liveHistoryStatus(row.Status) {
			// A live job with a reused engine ID supersedes its old terminal
			// record and suppression. Persist this before its next completion.
			if h.forgotten[row.GID] || h.rows[row.GID] != nil {
				delete(h.forgotten, row.GID)
				delete(h.rows, row.GID)
				changed = true
			}
		}
		if row.Status == "complete" || row.Status == "error" || row.Status == "removed" {
			var previous struct {
				Status     string
				RecordedAt int64 `json:"recordedAt"`
			}
			_ = json.Unmarshal(h.rows[row.GID], &previous)
			at := previous.RecordedAt
			if previous.Status != row.Status {
				at = time.Now().UnixMilli()
			}
			var record map[string]json.RawMessage
			if json.Unmarshal(raw, &record) != nil {
				continue
			}
			record["recordedAt"], _ = json.Marshal(at)
			stored, _ := json.Marshal(record)
			if !bytes.Equal(h.rows[row.GID], stored) {
				h.rows[row.GID] = stored
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
	if !fresh {
		return current, errHistoryObservationStale
	}
	if changed {
		if e := h.write(); e != nil {
			h.rows = before
			h.forgotten = beforeForgotten
			return current, e
		}
		h.generation++
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
	if !validHistoryID(gid) {
		return errors.New("invalid history identifier")
	}
	if !h.forgotten[gid] && len(h.forgotten) >= maxForgottenHistory {
		return errors.New("hidden-history limit reached; saved records kept")
	}
	before := maps.Clone(h.rows)
	forgotten, had := h.forgotten[gid]
	delete(h.rows, gid)
	h.forgotten[gid] = true
	if err := h.write(); err != nil {
		h.rows = before
		if had {
			h.forgotten[gid] = forgotten
		} else {
			delete(h.forgotten, gid)
		}
		return err
	}
	h.generation++
	return nil
}

func historyRecordedAt(raw json.RawMessage) int64 {
	var record struct {
		RecordedAt int64 `json:"recordedAt"`
	}
	_ = json.Unmarshal(raw, &record)
	return record.RecordedAt
}

func terminalHistoryStatus(status string) bool {
	return status == "complete" || status == "error" || status == "removed"
}

func liveHistoryStatus(status string) bool {
	return status == "active" || status == "paused" || status == "waiting" || status == "scheduled" || status == "seeding"
}
