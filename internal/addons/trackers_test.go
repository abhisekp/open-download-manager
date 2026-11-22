package addons

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func trackerManifestFixture() Manifest {
	m := manifest()
	m.API = 2
	m.Capability = "tracker-provider"
	m.OutputHosts = nil
	m.NetworkHosts = []string{"example.com"}
	m.FeedURL = "https://example.com/feed"
	return m
}

func TestTrackerProviderSignedManifestAndCapabilityBoundaries(t *testing.T) {
	m := trackerManifestFixture()
	key := signingKey(t)
	if _, err := Inspect(bundle(t, m, emptyModule, key)); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Manifest){
		func(m *Manifest) { m.API = 1 }, func(m *Manifest) { m.OutputHosts = []string{"example.com"} }, func(m *Manifest) { m.NetworkHosts = nil },
		func(m *Manifest) { m.FeedURL = "http://example.com/feed" }, func(m *Manifest) { m.FeedURL = "https://other.example/feed" },
		func(m *Manifest) { m.FeedURL = "https://user:pass@example.com/feed" }, func(m *Manifest) { m.FeedURL = "https://example.com/feed#private" },
		func(m *Manifest) { m.FeedURL = "https://example.com:8443/feed" }, func(m *Manifest) { m.FeedURL = "https://example.com/with space" },
	} {
		next := trackerManifestFixture()
		mutate(&next)
		if _, err := Inspect(bundle(t, next, emptyModule, key)); err == nil {
			t.Fatal("invalid signed tracker permission accepted", next)
		}
	}
	resolver := manifest()
	resolver.FeedURL = "https://example.com/feed"
	if _, err := Inspect(bundle(t, resolver, emptyModule, key)); err == nil {
		t.Fatal("resolver gained feed capability")
	}
}

func TestTrackerSuggestionValidationIsAtomicAndDistinctFromDownloads(t *testing.T) {
	m := trackerManifestFixture()
	raw := []byte(`{"trackers":["udp://tracker.example:6969/announce","https://tracker.example/announce","udp://tracker.example:6969/announce"]}`)
	got, err := decodeSuggestions(raw, m)
	if err != nil || len(got) != 2 {
		t.Fatal("deduplicated tracker suggestions failed", got, err)
	}
	empty, err := decodeSuggestions([]byte(`{"trackers":[]}`), m)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatal("empty feed not handled", err)
	}
	for _, invalid := range []string{`{"urls":["https://example.com/file"]}`, `{"trackers":null}`, `{"trackers":[],"urls":null}`, `{"trackers":[]} {}`, `{"trackers":["https://valid.example/announce","file:///private"]}`, `{"trackers":["udp://tracker.example/announce"]}`, `{"trackers":["https://user:secret@tracker.example/announce"]}`, `{"trackers":["https://tracker.example/announce#fragment"]}`, `{"trackers":["https://tracker.example:0/announce"]}`} {
		if _, err := decodeSuggestions([]byte(invalid), m); err == nil {
			t.Fatal("invalid response accepted", invalid)
		}
	}
	tooMany, _ := json.Marshal(map[string]any{"trackers": make([]string, 129)})
	if _, err := decodeSuggestions(tooMany, m); err == nil {
		t.Fatal("tracker count limit bypassed")
	}
	if _, err := decodeSuggestions(raw, manifest()); err == nil {
		t.Fatal("tracker output accepted as downloads")
	}
}

func TestTrackerProviderRealIsolatedWorkerAndApproval(t *testing.T) {
	module := compileExample(t, "./testdata/tracker-fixture")
	m := trackerManifestFixture()
	key := signingKey(t)
	raw := bundle(t, m, module, key)
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
	approval := ResolveApproval{Network: true, Digest: pkg.Digest}
	if _, err = store.FetchTrackers(context.Background(), m.ID, approval); err == nil {
		t.Fatal("disabled provider ran")
	}
	if err = store.SetEnabled(m.ID, true); err != nil {
		t.Fatal(err)
	}
	for _, a := range []ResolveApproval{{}, {Network: true}, {Network: true, Digest: strings.Repeat("0", 64)}} {
		if _, err = store.FetchTrackers(context.Background(), m.ID, a); err == nil {
			t.Fatal("provider approval/digest boundary failed")
		}
	}
	if _, err = store.ResolveApproved(context.Background(), m.ID, m.FeedURL, approval); err == nil {
		t.Fatal("tracker provider invoked as downloader")
	}
	got, err := store.FetchTrackers(context.Background(), m.ID, approval)
	if err != nil || len(got) != 2 {
		t.Fatal("real isolated tracker operation failed", got, err)
	}
	m.Version = "1.1.0"
	nextRaw := bundle(t, m, module, key)
	next, _ := Inspect(nextRaw)
	if _, err = store.Install(nextRaw, next.Digest); err != nil {
		t.Fatal(err)
	}
	if store.List()[0].Enabled {
		t.Fatal("update retained approval")
	}
	if err = store.SetEnabled(m.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err = store.FetchTrackers(context.Background(), m.ID, approval); err == nil {
		t.Fatal("old digest approved updated provider")
	}
	if err = store.Uninstall(m.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.FetchTrackers(context.Background(), m.ID, approval); err == nil {
		t.Fatal("uninstalled provider ran")
	}
}

func TestNewTrackonRealWASIWithPrivateTLSFixture(t *testing.T) {
	module := compileExample(t, "../../sdk/examples/newtrackon")
	var calls atomic.Int32
	client, _, _ := providerFixtureForHost(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/api/live" || r.URL.RawQuery != "" {
			t.Error("unexpected feed request")
		}
		_, _ = w.Write([]byte("udp://tracker.example:6969/announce\n\nhttps://tracker.example/announce\nudp://tracker.example:6969/announce\n"))
	}, "newtrackon.com")
	request, _ := json.Marshal(workerRequest{API: 2, Operation: "trackers", Module: module, URL: "https://newtrackon.com/api/live", NetworkHosts: []string{"newtrackon.com"}})
	var output, stderr limitedOutput
	if err := worker(bytes.NewReader(request), &output, &stderr, client); err != nil {
		t.Fatal("real feed WASI failed", err, stderr.String())
	}
	m := trackerManifestFixture()
	got, err := decodeSuggestions(output.Bytes(), m)
	if err != nil || calls.Load() != 1 || !reflect.DeepEqual(got, []string{"udp://tracker.example:6969/announce", "https://tracker.example/announce"}) {
		t.Fatal("feed/network/output failed", got, err)
	}
}
