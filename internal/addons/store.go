package addons

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/abhisekp/open-download-manager/internal/processutil"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

type Store struct {
	mu      sync.RWMutex
	dir     string
	entries map[string]Info
	gate    chan struct{}
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, entries: map[string]Info{}, gate: make(chan struct{}, 2)}
	file, err := os.Open(filepath.Join(dir, "registry.json"))
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var entries []Info
	if err = json.NewDecoder(io.LimitReader(file, 1<<20)).Decode(&entries); err != nil || len(entries) > 16 {
		return nil, errors.New("invalid add-on registry")
	}
	for _, entry := range entries {
		if entry.Manifest.Validate() != nil || len(entry.Digest) != 64 || !validDigest(entry.Digest) || s.entries[entry.Manifest.ID].Digest != "" {
			return nil, errors.New("invalid add-on registry entry")
		}
		pkg, e := s.load(entry.Digest)
		if e != nil || pkg.Manifest.ID != entry.Manifest.ID || pkg.Publisher != entry.Publisher {
			return nil, errors.New("add-on package failed integrity verification")
		}
		// Read permissions from the signed package, not mutable registry metadata.
		pkg.Enabled = entry.Enabled
		s.entries[pkg.Manifest.ID] = pkg.Info
	}
	return s, nil
}

func validDigest(value string) bool {
	for _, r := range value {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return len(value) == 64
}

func (s *Store) load(digest string) (*Package, error) {
	if !validDigest(digest) {
		return nil, errors.New("invalid package digest")
	}
	f, err := os.Open(filepath.Join(s.dir, digest+".odmx"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, MaxPackage+1))
	if err != nil {
		return nil, err
	}
	pkg, err := Inspect(raw)
	if err == nil && pkg.Digest != digest {
		err = errors.New("stored package digest mismatch")
	}
	return pkg, err
}

func (s *Store) List() []Info { s.mu.RLock(); defer s.mu.RUnlock(); return s.list() }
func (s *Store) list() []Info {
	rows := make([]Info, 0, len(s.entries))
	for _, entry := range s.entries {
		rows = append(rows, entry)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Manifest.ID < rows[j].Manifest.ID })
	return rows
}

func (s *Store) save() error {
	body, err := json.Marshal(s.list())
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(s.dir, ".registry-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err = temp.Chmod(0600); err == nil {
		_, err = temp.Write(body)
	}
	if err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(temp.Name(), filepath.Join(s.dir, "registry.json"))
}

func (s *Store) Install(raw []byte, approvedDigest string) (Info, error) {
	pkg, err := Inspect(raw)
	if err != nil {
		return Info{}, err
	}
	if approvedDigest != pkg.Digest {
		return Info{}, errors.New("review and approve this exact package digest")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, exists := s.entries[pkg.Manifest.ID]
	if exists && previous.Publisher != pkg.Publisher {
		return Info{}, errors.New("publisher key changed; uninstall before choosing a different publisher")
	}
	if exists && previous.Manifest.Version == pkg.Manifest.Version && previous.Digest != pkg.Digest {
		return Info{}, errors.New("version already installed with different content")
	}
	if !exists && len(s.entries) >= 16 {
		return Info{}, errors.New("maximum 16 installed add-ons")
	}
	path := filepath.Join(s.dir, pkg.Digest+".odmx")
	file, err := os.CreateTemp(s.dir, ".package-*")
	if err != nil {
		return Info{}, err
	}
	defer os.Remove(file.Name())
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(raw)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(file.Name(), path)
	}
	if err != nil {
		return Info{}, err
	}
	// Every install/update revokes the old enabled grant. New permissions require review.
	s.entries[pkg.Manifest.ID] = pkg.Info
	if err = s.save(); err != nil {
		if exists {
			s.entries[pkg.Manifest.ID] = previous
		} else {
			delete(s.entries, pkg.Manifest.ID)
		}
		return Info{}, err
	}
	if exists && previous.Digest != pkg.Digest {
		_ = os.Remove(filepath.Join(s.dir, previous.Digest+".odmx"))
	}
	return pkg.Info, nil
}

func (s *Store) SetEnabled(id string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, exists := s.entries[id]
	if !exists {
		return errors.New("unknown add-on")
	}
	next := previous
	next.Enabled = enabled
	s.entries[id] = next
	if err := s.save(); err != nil {
		s.entries[id] = previous
		return err
	}
	return nil
}

func (s *Store) Uninstall(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, exists := s.entries[id]
	if !exists {
		return errors.New("unknown add-on")
	}
	delete(s.entries, id)
	if err := s.save(); err != nil {
		s.entries[id] = previous
		return err
	}
	_ = os.Remove(filepath.Join(s.dir, previous.Digest+".odmx"))
	return nil
}

type limitedOutput struct{ bytes.Buffer }

func (b *limitedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 256<<10 {
		return 0, errors.New("add-on output exceeds 256 KiB")
	}
	return b.Buffer.Write(p)
}

func (s *Store) Resolve(ctx context.Context, id, input string) ([]string, error) {
	return s.ResolveApproved(ctx, id, input, ResolveApproval{})
}

type ResolveApproval struct {
	Network bool
	Digest  string
}

func (s *Store) ResolveApproved(ctx context.Context, id, input string, approval ResolveApproval) ([]string, error) {
	return s.executeApproved(ctx, id, input, approval, "url-resolver")
}

// FetchTrackers executes the fixed feed URL in the signed manifest. Results
// are suggestions only; this capability cannot create transfers/edit torrents.
func (s *Store) FetchTrackers(ctx context.Context, id string, approval ResolveApproval) ([]string, error) {
	return s.executeApproved(ctx, id, "", approval, "tracker-provider")
}

func (s *Store) executeApproved(ctx context.Context, id, input string, approval ResolveApproval, capability string) ([]string, error) {
	overall := 5 * time.Second
	s.mu.RLock()
	if s.entries[id].Manifest.API == 2 {
		overall = 12 * time.Second
	}
	s.mu.RUnlock()
	ctx, cancel := context.WithTimeout(ctx, overall)
	defer cancel()
	select {
	case s.gate <- struct{}{}:
		defer func() { <-s.gate }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.entries[id]
	if !ok || !entry.Enabled {
		return nil, errors.New("add-on is disabled or not installed")
	}
	if entry.Manifest.Capability != capability {
		return nil, errors.New("add-on capability does not match this operation")
	}
	if len(entry.Manifest.NetworkHosts) > 0 && (!approval.Network || approval.Digest != entry.Digest) {
		return nil, errors.New("network requests need approval for this exact add-on package; refresh and review its hosts again")
	}
	operation := "resolve"
	if capability == "tracker-provider" {
		input = entry.Manifest.FeedURL
		operation = "trackers"
	}
	if !permitted(input, entry.Manifest.InputHosts) {
		return nil, errors.New("input URL is outside approved hosts")
	}
	pkg, err := s.load(entry.Digest)
	if err != nil {
		return nil, errors.New("installed add-on failed integrity check")
	}
	// An isolated child owns the WASM runtime; an extension crash cannot corrupt
	// the core process. Only this executable may be launched, never package commands.
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(workerRequest{Module: pkg.Module, URL: input, API: entry.Manifest.API, NetworkHosts: entry.Manifest.NetworkHosts, Operation: operation})
	if err != nil {
		return nil, err
	}
	stdout, stderr := &limitedOutput{}, &limitedOutput{}
	command := exec.CommandContext(ctx, executable, "--addon-worker")
	processutil.Background(command)
	command.Stdin = bytes.NewReader(body)
	command.Stdout = stdout
	command.Stderr = stderr
	command.WaitDelay = 500 * time.Millisecond
	command.Env = []string{"GOMEMLIMIT=128MiB", "GORACE=atexit_sleep_ms=0"}
	if root := os.Getenv("SystemRoot"); root != "" {
		command.Env = append(command.Env, "SystemRoot="+root)
	}
	err = command.Run()
	if err != nil {
		return nil, fmt.Errorf("add-on execution failed or exceeded limits")
	}
	return decodeSuggestions(stdout.Bytes(), entry.Manifest)
}

type workerRequest struct {
	Module       []byte   `json:"module"`
	URL          string   `json:"url"`
	API          int      `json:"api,omitempty"`
	NetworkHosts []string `json:"network_hosts,omitempty"`
	Operation    string   `json:"operation,omitempty"`
}

// Worker provides WASI Preview 1 without mounts, environment or socket imports.
// API 2 may also use the reviewed, bounded public-HTTPS host capability.
// GOMEMLIMIT bounds GC targets, not total process RSS. Linear memory has a hard cap.
func Worker(input io.Reader, output, errorOutput io.Writer) error {
	return worker(input, output, errorOutput, nil)
}

func worker(input io.Reader, output, errorOutput io.Writer, client *http.Client) error {
	var request workerRequest
	d := json.NewDecoder(io.LimitReader(input, MaxPackage*4/3+16384))
	d.DisallowUnknownFields()
	if d.Decode(&request) != nil || d.Decode(new(any)) != io.EOF || len(request.Module) > MaxPackage || len(request.URL) > 8192 {
		return errors.New("invalid worker request")
	}
	if request.API == 0 {
		request.API = 1
	}
	if request.Operation == "" {
		request.Operation = "resolve"
	}
	if (request.Operation != "resolve" && request.Operation != "trackers") || (request.Operation == "trackers" && request.API != 2) {
		return errors.New("unsupported worker operation")
	}
	if (request.API != 1 && request.API != 2) || (request.API == 1 && len(request.NetworkHosts) > 0) || len(request.NetworkHosts) > 16 {
		return errors.New("unsupported worker capability")
	}
	seen := map[string]bool{}
	for _, host := range request.NetworkHosts {
		if len(host) > 253 || !hostname.MatchString(host) || strings.Contains(host, "..") || seen[host] {
			return errors.New("invalid worker network permission")
		}
		seen[host] = true
	}
	deadline := 2 * time.Second
	if request.API == 2 {
		deadline = 8 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	rt := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter().WithMemoryLimitPages(1024).WithCloseOnContextDone(true))
	defer rt.Close(context.Background())
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		return err
	}
	if request.API == 2 {
		if client == nil {
			client = providerClient(nil)
		}
		defer client.CloseIdleConnections()
		budget := &providerBudget{}
		_, err := rt.NewHostModuleBuilder("odm").NewFunctionBuilder().WithFunc(func(ctx context.Context, module api.Module, urlPointer, urlLength, outputPointer, capacity uint32) int32 {
			memory := module.Memory()
			if memory == nil || urlLength > 8192 || capacity == 0 || capacity > providerResponseLimit {
				return -1
			}
			raw, ok := memory.Read(urlPointer, urlLength)
			if !ok {
				return -1
			}
			if _, ok = memory.Read(outputPointer, capacity); !ok {
				return -1
			}
			body, err := budget.get(ctx, client, request.NetworkHosts, string(raw), capacity)
			if err != nil || !memory.Write(outputPointer, body) {
				return -1
			}
			return int32(len(body))
		}).Export("http_get").Instantiate(ctx)
		if err != nil {
			return err
		}
	}
	body, _ := json.Marshal(map[string]any{"api": request.API, "operation": request.Operation, "url": request.URL})
	_, err := rt.InstantiateWithConfig(ctx, request.Module, wazero.NewModuleConfig().WithStdin(bytes.NewReader(body)).WithStdout(output).WithStderr(errorOutput))
	return err
}
