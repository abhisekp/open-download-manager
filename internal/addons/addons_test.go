package addons

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func manifest() Manifest {
	return Manifest{API: 1, ID: "org.odm.test", Name: "Test", Version: "1.0.0", Capability: "url-resolver", InputHosts: []string{"example.com"}, OutputHosts: []string{"example.com"}}
}
func signingKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}
func bundle(t *testing.T, m Manifest, module []byte, key ed25519.PrivateKey) []byte {
	t.Helper()
	hash := sha256.Sum256(module)
	m.ModuleHash = hex.EncodeToString(hash[:])
	license := []byte("MIT original test fixture; generated WASI fixtures are used only for tests")
	licenseHash := sha256.Sum256(license)
	m.LicenseHash = hex.EncodeToString(licenseHash[:])
	raw, _ := json.Marshal(m)
	var body bytes.Buffer
	z := zip.NewWriter(&body)
	for _, f := range []struct {
		name string
		data []byte
	}{{"manifest.json", raw}, {"module.wasm", module}, {"publisher.ed25519", key.Public().(ed25519.PublicKey)}, {"signature.ed25519", ed25519.Sign(key, raw)}, {"LICENSE", license}} {
		w, _ := z.Create(f.name)
		_, _ = w.Write(f.data)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}

var emptyModule = []byte{0, 97, 115, 109, 1, 0, 0, 0}

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "--addon-worker" {
		if Worker(os.Stdin, os.Stdout, os.Stderr) != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestPackageSignatureAndEntryValidation(t *testing.T) {
	raw := bundle(t, manifest(), emptyModule, signingKey(t))
	pkg, err := Inspect(raw)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Publisher == "" || pkg.Digest == "" {
		t.Fatal("missing identity")
	}
	archive, _ := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	var modified bytes.Buffer
	out := zip.NewWriter(&modified)
	for _, entry := range archive.File {
		r, _ := entry.Open()
		body := new(bytes.Buffer)
		_, _ = body.ReadFrom(r)
		_ = r.Close()
		data := body.Bytes()
		if entry.Name == "module.wasm" {
			data = append(data, 0)
		}
		w, _ := out.Create(entry.Name)
		_, _ = w.Write(data)
	}
	_ = out.Close()
	if _, err = Inspect(modified.Bytes()); err == nil {
		t.Fatal("tampered module accepted")
	}
	var traversal bytes.Buffer
	out = zip.NewWriter(&traversal)
	_, _ = out.Create("../escape")
	_ = out.Close()
	if _, err = Inspect(traversal.Bytes()); err == nil {
		t.Fatal("path traversal accepted")
	}
}

func TestLifecycleAndPublisherContinuity(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	key := signingKey(t)
	raw := bundle(t, manifest(), emptyModule, key)
	pkg, _ := Inspect(raw)
	if _, err = s.Install(raw, ""); err == nil {
		t.Fatal("unapproved package installed")
	}
	if _, err = s.Install(raw, pkg.Digest); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Resolve(context.Background(), pkg.Manifest.ID, "https://example.com/f"); err == nil {
		t.Fatal("disabled extension executed")
	}
	if err = s.SetEnabled(pkg.Manifest.ID, true); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil || !s.List()[0].Enabled {
		t.Fatal("grant did not survive restart", err)
	}
	changed := manifest()
	changed.Version = "1.1.0"
	raw = bundle(t, changed, emptyModule, signingKey(t))
	pkg, _ = Inspect(raw)
	if _, err = s.Install(raw, pkg.Digest); err == nil {
		t.Fatal("publisher takeover accepted")
	}
	raw = bundle(t, changed, emptyModule, key)
	pkg, _ = Inspect(raw)
	if _, err = s.Install(raw, pkg.Digest); err != nil {
		t.Fatal(err)
	}
	if s.List()[0].Enabled {
		t.Fatal("update silently retained permission grant")
	}
	if err = s.Uninstall(changed.ID); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil || len(s.List()) != 0 {
		t.Fatal("uninstall not persisted", err)
	}
}

func compileExample(t *testing.T, source string) []byte {
	t.Helper()
	output := filepath.Join(t.TempDir(), "fixture.wasm")
	cmd := exec.Command("go", "build", "-o", output, source)
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0")
	if result, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("WASI fixture build: %v %s", err, result)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestRealWASIResolverAndHostPermissions(t *testing.T) {
	module := compileExample(t, "../../sdk/examples/clean-link")
	s, _ := Open(t.TempDir())
	m := manifest()
	key := signingKey(t)
	raw := bundle(t, m, module, key)
	pkg, _ := Inspect(raw)
	if _, err := s.Install(raw, pkg.Digest); err != nil {
		t.Fatal(err)
	}
	_ = s.SetEnabled(m.ID, true)
	urls, err := s.Resolve(context.Background(), m.ID, "https://example.com/file.zip?utm_source=test&keep=1#fragment")
	if err != nil || len(urls) != 1 || urls[0] != "https://example.com/file.zip?keep=1" {
		t.Fatalf("real WASI resolver: %v %v", urls, err)
	}
	if _, err = s.Resolve(context.Background(), m.ID, "https://not-approved.example/file"); err == nil {
		t.Fatal("unapproved input host accepted")
	}
	m.Version = "1.1.0"
	m.OutputHosts = []string{"other.example"}
	raw = bundle(t, m, module, key)
	pkg, _ = Inspect(raw)
	_, _ = s.Install(raw, pkg.Digest)
	_ = s.SetEnabled(m.ID, true)
	if _, err = s.Resolve(context.Background(), m.ID, "https://example.com/file"); err == nil || !strings.Contains(err.Error(), "outside approved") {
		t.Fatal("unapproved output host accepted", err)
	}
}

func TestRunawayModuleIsInterrupted(t *testing.T) {
	// (module (func (export "_start") (loop br 0)))
	module := []byte{0, 97, 115, 109, 1, 0, 0, 0, 1, 4, 1, 96, 0, 0, 3, 2, 1, 0, 7, 10, 1, 6, 95, 115, 116, 97, 114, 116, 0, 0, 10, 9, 1, 7, 0, 3, 64, 12, 0, 11, 11}
	s, _ := Open(t.TempDir())
	raw := bundle(t, manifest(), module, signingKey(t))
	pkg, err := Inspect(raw)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.Install(raw, pkg.Digest)
	_ = s.SetEnabled(pkg.Manifest.ID, true)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err = s.Resolve(ctx, pkg.Manifest.ID, "https://example.com/f"); err == nil {
		t.Fatal("runaway module accepted")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("runaway module escaped deadline")
	}
	if time.Since(start) < 50*time.Millisecond {
		t.Fatal("runaway fixture failed before its cancellation deadline")
	}
}

func TestWASISandboxHasNoFilesNetworkOrEnvironment(t *testing.T) {
	module := compileExample(t, "../../sdk/examples/sandbox-probe")
	file := filepath.Join(t.TempDir(), "private.txt")
	if err := os.WriteFile(file, []byte("private fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	t.Setenv("ODM_SANDBOX_PROBE", "private environment fixture")
	s, _ := Open(t.TempDir())
	raw := bundle(t, manifest(), module, signingKey(t))
	pkg, _ := Inspect(raw)
	_, _ = s.Install(raw, pkg.Digest)
	_ = s.SetEnabled(pkg.Manifest.ID, true)
	input := "https://example.com/probe?" + url.Values{"file": {file}, "socket": {listener.Addr().String()}}.Encode()
	urls, err := s.Resolve(context.Background(), pkg.Manifest.ID, input)
	if err != nil || len(urls) != 1 {
		t.Fatal("sandbox probe failed", urls, err)
	}
	if urls[0] != "https://example.com/probe?environment=empty&files=denied&network=denied" {
		t.Fatal("sandbox boundary failure", urls)
	}
}

func TestMemoryLimitAndStoredTampering(t *testing.T) {
	// A valid module with minimum memory 1025 pages exceeds the 1024-page limit.
	module := append(append([]byte{}, emptyModule...), 5, 4, 1, 0, 129, 8)
	s, _ := Open(t.TempDir())
	raw := bundle(t, manifest(), module, signingKey(t))
	pkg, _ := Inspect(raw)
	_, _ = s.Install(raw, pkg.Digest)
	_ = s.SetEnabled(pkg.Manifest.ID, true)
	if _, err := s.Resolve(context.Background(), pkg.Manifest.ID, "https://example.com/f"); err == nil || !strings.Contains(err.Error(), "execution failed") {
		t.Fatal("memory limit not enforced", err)
	}
	if err := os.WriteFile(filepath.Join(s.dir, pkg.Digest+".odmx"), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(context.Background(), pkg.Manifest.ID, "https://example.com/f"); err == nil || !strings.Contains(err.Error(), "integrity") {
		t.Fatal("stored tampering not detected", err)
	}
}
