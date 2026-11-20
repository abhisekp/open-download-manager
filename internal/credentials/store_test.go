package credentials

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var missing = errors.New("missing fixture key")

type fakeBackend struct {
	mu     sync.Mutex
	values map[string]string
	block  chan struct{}
	fail   bool
	sets   int
}

func (b *fakeBackend) Set(service, id, value string) error {
	if b.block != nil {
		<-b.block
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sets++
	if b.fail {
		return errors.New("private backend diagnostic")
	}
	b.values[service+id] = value
	return nil
}
func (b *fakeBackend) Get(service, id string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	v, ok := b.values[service+id]
	if !ok {
		return "", missing
	}
	return v, nil
}
func (b *fakeBackend) Delete(service, id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.values[service+id]; !ok {
		return missing
	}
	delete(b.values, service+id)
	return nil
}
func (*fakeBackend) Missing(err error) bool { return errors.Is(err, missing) }
func testStore(t *testing.T) (*Store, *fakeBackend) {
	t.Helper()
	backend := &fakeBackend{values: map[string]string{}}
	store, err := Open(filepath.Join(t.TempDir(), "credentials.json"), "owned-test-service", backend)
	if err != nil {
		t.Fatal(err)
	}
	return store, backend
}
func idle(t *testing.T, s *Store) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for len(s.gate) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("keyring operation did not finish")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestHTTPSOriginScope(t *testing.T) {
	for raw, want := range map[string]string{"https://EXAMPLE.test/path?query=yes": "https://example.test", "https://example.test:0443": "https://example.test", "https://[::1]:8443/file": "https://[::1]:8443"} {
		got, err := Origin(raw)
		if err != nil || got != want {
			t.Fatal("origin canonicalization failed")
		}
	}
	for _, raw := range []string{"http://example.test", "https://user:secret@example.test", "https://example.test:0", "https://example.test:65536", "https://例子.test", "file:///private"} {
		if _, err := Origin(raw); err == nil {
			t.Fatal("unsafe origin accepted")
		}
	}
}

func TestKeyringLifecycleNeverWritesPasswordOrUsername(t *testing.T) {
	s, b := testStore(t)
	entry, err := s.Create("Fixture account", "https://EXAMPLE.test/", "fixture-user", "fixture-private-password")
	if err != nil {
		t.Fatal(err)
	}
	idle(t, s)
	if s.List()[0].State != "ready" {
		t.Fatal("saved login not ready")
	}
	raw, err := os.ReadFile(s.filename)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"fixture-user", "fixture-private-password"} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatal("secret escaped into reference file")
		}
	}
	user, password, err := s.Lookup(context.Background(), entry.ID, "https://example.test/download.bin")
	if err != nil || user != "fixture-user" || password != "fixture-private-password" {
		t.Fatal("scoped lookup failed")
	}
	for _, raw := range []string{"http://example.test/file", "https://other.test/file", "https://sub.example.test/file", "https://example.test:8443/file"} {
		if _, _, err = s.Lookup(context.Background(), entry.ID, raw); err == nil {
			t.Fatal("credential escaped its origin")
		}
	}
	reopened, err := Open(s.filename, s.service, b)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = reopened.Lookup(context.Background(), entry.ID, "https://example.test/next"); err != nil {
		t.Fatal("keyring reference did not survive restart")
	}
	if err = reopened.Act(entry.ID, "remove"); err != nil {
		t.Fatal(err)
	}
	idle(t, reopened)
	if len(reopened.List()) != 0 {
		t.Fatal("reference not removed")
	}
	b.mu.Lock()
	remaining := len(b.values)
	b.mu.Unlock()
	if remaining != 0 {
		t.Fatal("exact owned OS key not deleted")
	}
}

func TestPendingSaveRecoveryAndBoundedOSOperations(t *testing.T) {
	s, b := testStore(t)
	b.block = make(chan struct{})
	entry, err := s.Create("Pending", "https://example.test", "user", "password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Create("Other", "https://example.test", "user", "password"); !errors.Is(err, ErrBusy) {
		t.Fatal("unbounded OS write operations accepted")
	}
	if err = s.Act(entry.ID, "remove"); !errors.Is(err, ErrBusy) {
		t.Fatal("removal raced a pending write")
	}
	// Preserve the journal while simulating failure to save the completion.
	filename := s.filename
	s.mu.Lock()
	s.filename = filepath.Join(t.TempDir(), "absent", "state.json")
	s.mu.Unlock()
	close(b.block)
	idle(t, s)
	reopened, err := Open(filename, s.service, b)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.List()[0].State != "pending" {
		t.Fatal("pending journal lost")
	}
	if _, _, err = reopened.Lookup(context.Background(), entry.ID, "https://example.test/file"); !errors.Is(err, ErrMissing) {
		t.Fatal("pending login became usable without recovery")
	}
	if err = reopened.Act(entry.ID, "check"); err != nil {
		t.Fatal(err)
	}
	idle(t, reopened)
	if reopened.List()[0].State != "ready" {
		t.Fatal("existing OS key did not reconcile")
	}
	if err = reopened.Act(entry.ID, "remove"); err != nil {
		t.Fatal(err)
	}
	idle(t, reopened)
}

func TestCredentialInputAndFailedWriteHaveNoSecretSideEffects(t *testing.T) {
	s, b := testStore(t)
	for _, tc := range [][4]string{{"Label", "http://example.test", "u", "p"}, {"Label", "https://example.test/path", "u", "p"}, {"Label", "https://example.test/?secret=x", "u", "p"}, {"Label", "https://example.test", "a:b", "p"}, {"Label", "https://example.test", "u\n", "p"}, {"Label", "https://example.test", "u", strings.Repeat("x", 769)}} {
		if _, err := s.Create(tc[0], tc[1], tc[2], tc[3]); err == nil {
			t.Fatal("invalid credential input accepted")
		}
	}
	s.filename = filepath.Join(t.TempDir(), "absent", "state.json")
	if _, err := s.Create("Label", "https://example.test", "u", "p"); !errors.Is(err, ErrStorage) {
		t.Fatal("failed journal write accepted")
	}
	b.mu.Lock()
	sets := b.sets
	b.mu.Unlock()
	if sets != 0 || len(s.List()) != 0 {
		t.Fatal("secret write occurred without a durable reference")
	}
}

func TestOriginTamperingAndCorruptRegistryFailClosed(t *testing.T) {
	s, b := testStore(t)
	entry, err := s.Create("Scope", "https://example.test", "user", "password")
	if err != nil {
		t.Fatal(err)
	}
	idle(t, s)
	s.mu.Lock()
	modified := s.entries[entry.ID]
	modified.Origin = "https://other.test"
	s.entries[entry.ID] = modified
	delete(s.cache, entry.ID)
	s.mu.Unlock()
	if _, _, err = s.Lookup(context.Background(), entry.ID, "https://other.test/file"); !errors.Is(err, ErrScope) {
		t.Fatal("metadata rewrite redirected an OS secret")
	}
	bad := []byte(`{"version":1,"entries":[],"unexpected":true}`)
	if os.WriteFile(s.filename, bad, 0600) != nil {
		t.Fatal("fixture write")
	}
	if _, err = Open(s.filename, s.service, b); err == nil {
		t.Fatal("corrupt reference registry accepted")
	}
	after, _ := os.ReadFile(s.filename)
	if !bytes.Equal(after, bad) {
		t.Fatal("corrupt registry bytes rewritten")
	}
}

func TestCloseKeepsPendingReferenceAndRejectsLateMetadataWrites(t *testing.T) {
	s, b := testStore(t)
	b.block = make(chan struct{})
	entry, err := s.Create("Closing", "https://example.test", "user", "password")
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.filename)
	s.Close()
	close(b.block)
	idle(t, s)
	after, _ := os.ReadFile(s.filename)
	if !bytes.Equal(before, after) {
		t.Fatal("late keyring result wrote metadata after Close")
	}
	if _, _, err = s.Lookup(context.Background(), entry.ID, "https://example.test/file"); err == nil {
		t.Fatal("closed store exposed a secret")
	}
	reopened, err := Open(s.filename, s.service, b)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err = reopened.Act(entry.ID, "check"); err != nil {
		t.Fatal(err)
	}
	idle(t, reopened)
	if reopened.List()[0].State != "ready" {
		t.Fatal("late successful save was not recoverable")
	}
	if err = reopened.Act(entry.ID, "remove"); err != nil {
		t.Fatal(err)
	}
	idle(t, reopened)
}
