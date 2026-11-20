package addons

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAPI2SignedPermissionAndExactPackageApproval(t *testing.T) {
	m := manifest()
	m.NetworkHosts = []string{"httpbin.org"}
	legacy := bundle(t, m, emptyModule, signingKey(t))
	if _, err := Inspect(legacy); err == nil || !strings.Contains(err.Error(), "API 2") {
		t.Fatal("API 1 silently gained network permission")
	}
	m.API = 2
	key := signingKey(t)
	raw := bundle(t, m, emptyModule, key)
	pkg, err := Inspect(raw)
	if err != nil {
		t.Fatal(err)
	}
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Install(raw, pkg.Digest); err != nil {
		t.Fatal(err)
	}
	if err = store.SetEnabled(m.ID, true); err != nil {
		t.Fatal(err)
	}
	for _, approval := range []ResolveApproval{{}, {Network: true}, {Network: true, Digest: strings.Repeat("0", 64)}} {
		if _, err = store.ResolveApproved(context.Background(), m.ID, "https://example.com/synthetic.bin", approval); err == nil || !strings.Contains(err.Error(), "exact add-on package") {
			t.Fatal("network approval/digest boundary failed", err)
		}
	}
	m.Version = "1.1.0"
	m.NetworkHosts = []string{"other.example.test"}
	updated := bundle(t, m, emptyModule, key)
	next, _ := Inspect(updated)
	if _, err = store.Install(updated, next.Digest); err != nil {
		t.Fatal(err)
	}
	if store.List()[0].Enabled {
		t.Fatal("network update retained enabled grant")
	}
	if err = store.SetEnabled(m.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err = store.ResolveApproved(context.Background(), m.ID, "https://example.com/synthetic.bin", ResolveApproval{Network: true, Digest: pkg.Digest}); err == nil {
		t.Fatal("old package consent authorized new network hosts")
	}
}

func TestAPI2ApprovedIsolatedWorkerWithoutUnreviewedNetwork(t *testing.T) {
	module := compileExample(t, "../../sdk/examples/network-probe")
	m := manifest()
	m.API = 2
	m.NetworkHosts = []string{"httpbin.org"}
	raw := bundle(t, m, module, signingKey(t))
	pkg, err := Inspect(raw)
	if err != nil {
		t.Fatal(err)
	}
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Install(raw, pkg.Digest); err != nil {
		t.Fatal(err)
	}
	if err = store.SetEnabled(m.ID, true); err != nil {
		t.Fatal(err)
	}
	// The real child imports the new host ABI but must refuse this destination
	// before DNS/dialing. The fixture then suggests the unchanged reviewed input.
	input := "https://example.com/probe?mode=resolve&target=" + url.QueryEscape("https://not-declared.invalid/fixture")
	urls, err := store.ResolveApproved(context.Background(), m.ID, input, ResolveApproval{Network: true, Digest: pkg.Digest})
	if err != nil || len(urls) != 1 || urls[0] != input {
		t.Fatal("approved isolated API 2 worker or host refusal failed", err)
	}
}

func TestRealAPI2WASIProviderAndOutput(t *testing.T) {
	module := compileExample(t, "../../sdk/examples/network-echo")
	var requests atomic.Int32
	client, _, _ := providerFixtureForHost(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/anything" {
			t.Error("unexpected provider path")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"args": map[string]string{"url": r.URL.Query().Get("url")}})
	}, "httpbin.org")
	input := "https://example.com/original.bin?synthetic=1"
	request, _ := json.Marshal(workerRequest{API: 2, Module: module, URL: input, NetworkHosts: []string{"httpbin.org"}})
	var output, stderr limitedOutput
	if err := worker(bytes.NewReader(request), &output, &stderr, client); err != nil {
		t.Fatal("real API 2 module failed", err, stderr.String())
	}
	var result struct {
		URLs []string `json:"urls"`
	}
	if json.Unmarshal(output.Bytes(), &result) != nil || len(result.URLs) != 1 || result.URLs[0] != input || requests.Load() != 1 {
		t.Fatal("real API 2 request/response or source integrity failed")
	}
	request, _ = json.Marshal(workerRequest{API: 1, Module: module, URL: input})
	if err := Worker(bytes.NewReader(request), &limitedOutput{}, &limitedOutput{}); err == nil {
		t.Fatal("API 1 instantiated a network import")
	}
}

func TestAPI2WASIMemoryAndRequestBoundaries(t *testing.T) {
	module := compileExample(t, "../../sdk/examples/network-probe")
	var requests atomic.Int32
	client, _, _ := providerFixtureForHost(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/large" {
			_, _ = w.Write([]byte(strings.Repeat("x", providerResponseLimit)))
		} else {
			_, _ = w.Write([]byte("original"))
		}
	}, "httpbin.org")
	run := func(query string) []int32 {
		request, _ := json.Marshal(workerRequest{API: 2, Module: module, URL: "https://example.com/probe?" + query, NetworkHosts: []string{"httpbin.org"}})
		var output, stderr limitedOutput
		if err := worker(bytes.NewReader(request), &output, &stderr, client); err != nil {
			t.Fatal("probe module failed", err)
		}
		var result struct {
			Results []int32 `json:"results"`
		}
		if json.Unmarshal(output.Bytes(), &result) != nil {
			t.Fatal("invalid probe output")
		}
		return result.Results
	}
	for _, mode := range []string{"bad-input", "bad-output", "large-input", "large-output", "empty-output"} {
		before := requests.Load()
		result := run("mode=" + mode)
		if len(result) != 1 || result[0] != -1 || requests.Load() != before {
			t.Fatal("invalid WASM range reached network", mode)
		}
	}
	before := requests.Load()
	result := run("attempts=5")
	if len(result) != 5 || result[4] != -1 || requests.Load()-before != 4 {
		t.Fatal("WASI attempt budget bypassed")
	}
	before = requests.Load()
	result = run("attempts=3&target=" + url.QueryEscape("https://httpbin.org/large"))
	if len(result) != 3 || result[0] != providerResponseLimit || result[1] != providerResponseLimit || result[2] != -1 || requests.Load()-before != 2 {
		t.Fatal("WASI total response budget bypassed")
	}
}
