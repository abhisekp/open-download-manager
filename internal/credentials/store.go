// Package credentials stores only account references in application data.
// Password payloads belong to the operating-system keyring, never JSON files.
package credentials

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrBusy        = errors.New("system keyring operation already in progress")
	ErrUnavailable = errors.New("system keyring unavailable or locked")
	ErrMissing     = errors.New("saved login unavailable; check it in Settings")
	ErrScope       = errors.New("saved login is restricted to its HTTPS website")
	ErrStorage     = errors.New("cannot save login references; check application data folder access")
	ErrInput       = errors.New("provide a label, HTTPS website, username without colons, and supported password")
)

type Backend interface {
	Set(service, id, value string) error
	Get(service, id string) (string, error)
	Delete(service, id string) error
	Missing(error) bool
}

type Entry struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Origin string `json:"origin"`
	State  string `json:"state"`
	Action string `json:"action"`
}
type payload struct {
	Origin   string `json:"origin"`
	Username string `json:"username"`
	Password string `json:"password"`
}
type registry struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}
type cached struct {
	secret payload
	until  time.Time
}

type Store struct {
	mu                sync.Mutex
	filename, service string
	backend           Backend
	entries           map[string]Entry
	cache             map[string]cached
	gate              chan struct{}
	closed            bool
}

// Origin deliberately accepts only HTTPS and ASCII hostnames. Redirects never
// expand a credential's scope to another host, port, or insecure scheme.
func Origin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Hostname() == "" {
		return "", ErrScope
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if len(host) > 253 {
		return "", ErrScope
	}
	for _, r := range host {
		if r > 127 || r < 33 {
			return "", ErrScope
		}
	}
	port := u.Port()
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", ErrScope
		}
		port = strconv.Itoa(n)
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port != "" && port != "443" {
		host += ":" + port
	}
	return "https://" + host, nil
}

func validID(id string) bool {
	if len(id) != 32 || id != strings.ToLower(id) {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}
func clean(value string, max int) bool {
	return value != "" && len(value) <= max && utf8.ValidString(value) && strings.IndexFunc(value, unicode.IsControl) < 0
}
func validSecret(p payload) bool {
	origin, err := Origin(p.Origin)
	return err == nil && origin == p.Origin && clean(p.Username, 128) && !strings.Contains(p.Username, ":") && clean(p.Password, 768)
}

func Open(filename, service string, backend Backend) (*Store, error) {
	if backend == nil || !clean(service, 128) {
		return nil, ErrInput
	}
	m := &Store{filename: filename, service: service, backend: backend, entries: map[string]Entry{}, cache: map[string]cached{}, gate: make(chan struct{}, 1)}
	info, err := os.Lstat(filename)
	if os.IsNotExist(err) {
		return m, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > 128<<10 {
		return nil, ErrStorage
	}
	f, err := os.Open(filename)
	if err != nil {
		return nil, ErrStorage
	}
	defer f.Close()
	var state registry
	d := json.NewDecoder(io.LimitReader(f, 128<<10))
	d.DisallowUnknownFields()
	if d.Decode(&state) != nil || d.Decode(new(any)) != io.EOF || state.Version != 1 || len(state.Entries) > 30 {
		return nil, ErrStorage
	}
	for _, entry := range state.Entries {
		origin, err := Origin(entry.Origin)
		if err != nil || origin != entry.Origin || !validID(entry.ID) || !clean(entry.Label, 128) || m.entries[entry.ID].ID != "" {
			return nil, ErrStorage
		}
		if entry.State != "ready" && entry.State != "pending" && entry.State != "deleting" && entry.State != "error" {
			return nil, ErrStorage
		}
		if entry.Action != "save" && entry.Action != "remove" && entry.Action != "check" {
			return nil, ErrStorage
		}
		m.entries[entry.ID] = entry
	}
	return m, nil
}

func (m *Store) listLocked() []Entry {
	rows := make([]Entry, 0, len(m.entries))
	for _, e := range m.entries {
		rows = append(rows, e)
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Label < rows[j].Label || rows[i].Label == rows[j].Label && rows[i].ID < rows[j].ID
	})
	return rows
}
func (m *Store) List() []Entry { m.mu.Lock(); defer m.mu.Unlock(); return m.listLocked() }
func (m *Store) Busy() bool    { return len(m.gate) != 0 }
func (m *Store) saveLocked() error {
	data, err := json.Marshal(registry{1, m.listLocked()})
	if err != nil {
		return ErrStorage
	}
	f, err := os.CreateTemp(filepath.Dir(m.filename), ".login-reference-*")
	if err != nil {
		return ErrStorage
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil || os.Rename(f.Name(), m.filename) != nil {
		return ErrStorage
	}
	return nil
}

func (m *Store) acquire() bool {
	select {
	case m.gate <- struct{}{}:
		return true
	default:
		return false
	}
}
func (m *Store) release() { <-m.gate }

// Create first journals a pending reference. A late keyring result therefore
// remains recoverable by this ID even when a window closes or the app restarts.
// Only one OS operation can exist at a time; a locked keyring cannot spawn an
// unbounded pile of waiting helper processes.
func (m *Store) Create(label, rawOrigin, username, password string) (Entry, error) {
	u, err := url.Parse(rawOrigin)
	if err != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return Entry{}, ErrInput
	}
	origin, err := Origin(rawOrigin)
	if err != nil {
		return Entry{}, ErrInput
	}
	p := payload{origin, username, password}
	secret, err := json.Marshal(p)
	if err != nil || !validSecret(p) || !clean(label, 128) || len(secret) > 1400 {
		return Entry{}, ErrInput
	}
	if !m.acquire() {
		return Entry{}, ErrBusy
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		m.release()
		return Entry{}, ErrUnavailable
	}
	if len(m.entries) >= 30 {
		m.mu.Unlock()
		m.release()
		return Entry{}, errors.New("saved login limit reached (30)")
	}
	for _, existing := range m.entries {
		if existing.Origin == origin && strings.EqualFold(existing.Label, label) {
			m.mu.Unlock()
			m.release()
			return Entry{}, errors.New("choose a different login label for this website")
		}
	}
	key := make([]byte, 16)
	if _, err = rand.Read(key); err != nil {
		m.mu.Unlock()
		m.release()
		return Entry{}, ErrUnavailable
	}
	id := hex.EncodeToString(key)
	if m.entries[id].ID != "" {
		m.mu.Unlock()
		m.release()
		return Entry{}, ErrUnavailable
	}
	entry := Entry{id, label, origin, "pending", "save"}
	m.entries[id] = entry
	if err = m.saveLocked(); err != nil {
		delete(m.entries, id)
		m.mu.Unlock()
		m.release()
		return Entry{}, err
	}
	m.mu.Unlock()
	go func() {
		defer m.release()
		opErr := m.backend.Set(m.service, id, string(secret))
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.closed {
			return
		}
		previous := m.entries[id]
		next := previous
		if opErr == nil {
			next.State = "ready"
		} else {
			next.State = "error"
		}
		m.entries[id] = next
		if m.saveLocked() != nil {
			m.entries[id] = previous
			return
		}
		if opErr == nil {
			m.cache[id] = cached{p, time.Now().Add(5 * time.Minute)}
		}
	}()
	return entry, nil
}

// Act removes an exact owned key or explicitly reconciles a pending reference.
// No OS-wide listing or DeleteAll operation is exposed.
func (m *Store) Act(id, action string) error {
	if action != "remove" && action != "check" {
		return ErrInput
	}
	if !m.acquire() {
		return ErrBusy
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		m.release()
		return ErrUnavailable
	}
	entry, ok := m.entries[id]
	if !ok {
		m.mu.Unlock()
		m.release()
		return ErrMissing
	}
	if action == "check" && entry.Action == "remove" {
		m.mu.Unlock()
		m.release()
		return ErrInput
	}
	previous := entry
	entry.Action = action
	entry.State = "pending"
	if action == "remove" {
		entry.State = "deleting"
	}
	m.entries[id] = entry
	delete(m.cache, id)
	if err := m.saveLocked(); err != nil {
		m.entries[id] = previous
		m.mu.Unlock()
		m.release()
		return err
	}
	m.mu.Unlock()
	go func() {
		defer m.release()
		var opErr error
		var secret payload
		if action == "remove" {
			opErr = m.backend.Delete(m.service, id)
			if m.backend.Missing(opErr) {
				opErr = nil
			}
		} else {
			secret, opErr = m.readSecret(id, entry.Origin)
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.closed {
			return
		}
		before := m.entries[id]
		if action == "remove" && opErr == nil {
			delete(m.entries, id)
		} else {
			next := before
			next.State = "ready"
			if opErr != nil {
				next.State = "error"
			}
			m.entries[id] = next
		}
		if m.saveLocked() != nil {
			m.entries[id] = before
			return
		}
		if action == "check" && opErr == nil {
			m.cache[id] = cached{secret, time.Now().Add(5 * time.Minute)}
		}
	}()
	return nil
}

func (m *Store) readSecret(id, origin string) (payload, error) {
	raw, err := m.backend.Get(m.service, id)
	if err != nil {
		return payload{}, ErrUnavailable
	}
	if len(raw) > 1400 {
		return payload{}, ErrUnavailable
	}
	var p payload
	d := json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&p) != nil || d.Decode(new(any)) != io.EOF || !validSecret(p) || p.Origin != origin {
		return payload{}, ErrScope
	}
	return p, nil
}

func (m *Store) Validate(id, rawURL string) error {
	origin, err := Origin(rawURL)
	if err != nil {
		return ErrScope
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrMissing
	}
	entry, ok := m.entries[id]
	if !ok || entry.State != "ready" {
		return ErrMissing
	}
	if entry.Origin != origin {
		return ErrScope
	}
	return nil
}

func (m *Store) Lookup(ctx context.Context, id, rawURL string) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if ctx.Err() != nil {
		return "", "", ctx.Err()
	}
	if err := m.Validate(id, rawURL); err != nil {
		return "", "", err
	}
	origin, _ := Origin(rawURL)
	m.mu.Lock()
	value, ok := m.cache[id]
	m.mu.Unlock()
	if ok && time.Now().Before(value.until) && value.secret.Origin == origin {
		return value.secret.Username, value.secret.Password, nil
	}
	// Reads are bounded by the caller and leave at most one OS read in flight.
	select {
	case m.gate <- struct{}{}:
	case <-ctx.Done():
		return "", "", ctx.Err()
	}
	if err := m.Validate(id, rawURL); err != nil {
		m.release()
		return "", "", err
	}
	type result struct {
		secret payload
		err    error
	}
	done := make(chan result, 1)
	go func() { defer m.release(); p, err := m.readSecret(id, origin); done <- result{p, err} }()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return "", "", ctx.Err()
	case <-timer.C:
		return "", "", ErrUnavailable
	case result := <-done:
		if result.err != nil {
			return "", "", result.err
		}
		m.mu.Lock()
		entry, ok := m.entries[id]
		if m.closed || !ok || entry.State != "ready" || entry.Origin != origin {
			m.mu.Unlock()
			return "", "", ErrMissing
		}
		m.cache[id] = cached{result.secret, time.Now().Add(5 * time.Minute)}
		m.mu.Unlock()
		return result.secret.Username, result.secret.Password, nil
	}
}

// Close prevents late keyring results from writing metadata after application
// data ownership is released. Pending references remain available for recovery.
func (m *Store) Close() { m.mu.Lock(); m.closed = true; m.cache = map[string]cached{}; m.mu.Unlock() }
