package transfers

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func authConfig(t *testing.T, origin string, client *http.Client) Config {
	t.Helper()
	parsed, _ := url.Parse(origin)
	scope := requestOrigin(parsed)
	validate := func(ref, raw string) error {
		u, err := url.Parse(raw)
		if err != nil || requestOrigin(u) != scope || ref != strings.Repeat("a", 32) && ref != strings.Repeat("b", 32) {
			return errors.New("not approved")
		}
		return nil
	}
	return Config{DataDir: filepath.Join(testDir(t), "data"), Downloads: filepath.Join(testDir(t), "files"), Client: client, ValidateCredential: validate, LookupCredential: func(ctx context.Context, ref, raw string) (string, string, error) {
		if err := validate(ref, raw); err != nil {
			return "", "", err
		}
		if ref == strings.Repeat("b", 32) {
			return "second-user", "second-fixture-password", nil
		}
		return "first-user", "first-fixture-password", nil
	}}
}

func authManager(t *testing.T, cfg Config) *Manager {
	t.Helper()
	m, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
		cleanupTestChild(t, cfg.DataDir)
		cleanupTestChild(t, cfg.Downloads)
	})
	return m
}

func authenticatedRange(w http.ResponseWriter, r *http.Request, payload []byte, hold bool) {
	start, end := int64(0), int64(len(payload)-1)
	if header := r.Header.Get("Range"); header != "" {
		if _, err := fmt.Sscanf(header, "bytes=%d-%d", &start, &end); err != nil || start < 0 || end >= int64(len(payload)) || end < start {
			w.WriteHeader(416)
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
	}
	w.Header().Set("ETag", `"shared-by-accounts"`)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Length", fmt.Sprint(end-start+1))
	if r.Header.Get("Range") != "" {
		w.WriteHeader(206)
	}
	for offset := start; offset <= end; {
		n := min(int64(8192), end-offset+1)
		if _, err := w.Write(payload[offset : offset+n]); err != nil {
			return
		}
		offset += n
		w.(http.Flusher).Flush()
		if hold && offset-start >= 128<<10 {
			<-r.Context().Done()
			return
		}
	}
}

func TestHTTPSBasicParallelTransferAndNoSecretPersistence(t *testing.T) {
	payload := segmentPayload()
	var approved, rejected atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "first-user" || p != "first-fixture-password" {
			rejected.Add(1)
			w.WriteHeader(401)
			return
		}
		approved.Add(1)
		authenticatedRange(w, r, payload, false)
	}))
	t.Cleanup(server.Close)
	cfg := authConfig(t, server.URL, server.Client())
	m := authManager(t, cfg)
	id, err := m.AddWithOptions(server.URL+"/private.bin", AddOptions{CredentialRef: strings.Repeat("a", 32)})
	if err != nil {
		t.Fatal(err)
	}
	job := complete(t, m, id)
	if rejected.Load() != 0 || approved.Load() < 4 || len(job.Segments) != 3 || !bytes.Equal(content(t, cfg, job), payload) {
		t.Fatal("authenticated range transfer failed")
	}
	raw, _ := os.ReadFile(filepath.Join(cfg.DataDir, "http-transfers.json"))
	for _, s := range []string{"first-user", "first-fixture-password", "Basic "} {
		if bytes.Contains(raw, []byte(s)) {
			t.Fatal("authentication data escaped into transfer state")
		}
	}
	if _, err = m.AddWithOptions(strings.Replace(server.URL, "https:", "http:", 1)+"/bad", AddOptions{CredentialRef: strings.Repeat("a", 32)}); err == nil {
		t.Fatal("saved login accepted over HTTP")
	}
}

func TestRedirectNeverForwardsLoginToAnotherOrigin(t *testing.T) {
	var leaked, received atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		if r.Header.Get("Authorization") != "" {
			leaked.Add(1)
		}
		_, _ = w.Write([]byte("public redirected bytes"))
	}))
	t.Cleanup(target.Close)
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "first-user" || p != "first-fixture-password" {
			w.WriteHeader(401)
			return
		}
		http.Redirect(w, r, target.URL+"/public", 302)
	}))
	t.Cleanup(source.Close)
	pool := x509.NewCertPool()
	pool.AddCert(source.Certificate())
	pool.AddCert(target.Certificate())
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}}
	cfg := authConfig(t, source.URL, client)
	m := authManager(t, cfg)
	if _, err := m.AddWithOptions(target.URL+"/forbidden", AddOptions{CredentialRef: strings.Repeat("a", 32)}); err == nil {
		t.Fatal("account reference assigned to different port/origin")
	}
	id, err := m.AddWithOptions(source.URL+"/redirect", AddOptions{CredentialRef: strings.Repeat("a", 32)})
	if err != nil {
		t.Fatal(err)
	}
	job := complete(t, m, id)
	if leaked.Load() != 0 || received.Load() != 1 || string(content(t, cfg, job)) != "public redirected bytes" {
		t.Fatal("cross-origin redirect carried credentials or failed safe public response")
	}
}

func TestChangingAccountDiscardsOldPartialEvenWhenETagIsReused(t *testing.T) {
	payload := segmentPayload()
	other := bytes.Repeat([]byte{'N'}, len(payload))
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok {
			w.WriteHeader(401)
			return
		}
		switch {
		case u == "first-user" && p == "first-fixture-password":
			authenticatedRange(w, r, payload, true)
		case u == "second-user" && p == "second-fixture-password":
			authenticatedRange(w, r, other, false)
		default:
			w.WriteHeader(401)
		}
	}))
	t.Cleanup(server.Close)
	cfg := authConfig(t, server.URL, server.Client())
	m := authManager(t, cfg)
	id, err := m.AddWithOptions(server.URL+"/account.bin", AddOptions{CredentialRef: strings.Repeat("a", 32)})
	if err != nil {
		t.Fatal(err)
	}
	await(t, m, id, func(j Job) bool { return len(j.Segments) == 3 && j.Completed >= 128<<10 })
	if err = m.SetCredential(id, strings.Repeat("b", 32)); err == nil {
		t.Fatal("active account changed while writers were running")
	}
	if err = m.Control(id, "pause"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		m.mu.Lock()
		running := m.running[id] != nil
		m.mu.Unlock()
		if !running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("pause did not stop writers")
		}
		time.Sleep(time.Millisecond)
	}
	if err = m.SetCredential(id, strings.Repeat("b", 32)); err != nil {
		t.Fatal(err)
	}
	job := m.Jobs()[0]
	if job.Completed != 0 || job.Committed != 0 || len(job.Segments) != 0 || job.ETag != "" || job.Status != "paused" {
		t.Fatal("account change reused old byte validators")
	}
	if err = m.Control(id, "unpause"); err != nil {
		t.Fatal(err)
	}
	job = complete(t, m, id)
	if !bytes.Equal(content(t, cfg, job), other) {
		t.Fatal("different accounts were combined into one download")
	}
}
