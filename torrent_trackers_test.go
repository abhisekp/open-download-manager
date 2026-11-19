package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zeebo/bencode"
)

func trackerFixture(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	count := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		body, _ := bencode.EncodeBytes(map[string]any{"interval": 60, "peers": ""})
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return server, count
}

func addTrackerFixtureTorrent(t *testing.T, native *NativeTorrents, private bool) string {
	t.Helper()
	data, _ := fixtureTorrent(t, "tracker-test.bin", []byte("independent tracker test fixture"))
	if private {
		var meta map[string]any
		if err := bencode.DecodeBytes(data, &meta); err != nil {
			t.Fatal(err)
		}
		meta["info"].(map[string]any)["private"] = int64(1)
		var err error
		data, err = bencode.EncodeBytes(meta)
		if err != nil {
			t.Fatal(err)
		}
	}
	raw, err := native.addFile(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	var gid string
	if json.Unmarshal(raw, &gid) != nil {
		t.Fatal("invalid fixture id")
	}
	return gid
}

func TestTrackerURLTrustBoundary(t *testing.T) {
	for _, raw := range []string{"https://example.test/announce?key=chosen", "http://127.0.0.1:8431/announce", "udp://[::1]:6969/announce"} {
		if _, err := trackerURL(raw); err != nil {
			t.Errorf("rejected supported tracker: %v", err)
		}
	}
	for _, raw := range []string{"", "file:///tmp/tracker", "javascript:alert(1)", "udp://example.test/announce", "udp://example.test:0/announce", "http://user:secret@example.test/announce", "https://example.test/announce#fragment", "http://example.test:65536", "http://example.test:abc", "http://example.test/a\nb", " https://example.test/announce", "https://example.test/" + strings.Repeat("a", 2048)} {
		if _, err := trackerURL(raw); err == nil {
			t.Error("accepted invalid tracker URL")
		}
	}
}

func TestTrackerBatchReportsCommittedPrefixOnStorageFailure(t *testing.T) {
	var attempted []string
	result := persistTrackerBatch([]string{"one", "two", "three"}, func(value string) error {
		attempted = append(attempted, value)
		if value == "two" {
			return errors.New("private storage failure detail")
		}
		return nil
	})
	if strings.Join(attempted, ",") != "one,two" || strings.Join(result.Added, ",") != "one" || result.Remaining != 2 || result.Error == "" {
		t.Fatal("partial persistence was misreported or later writes continued")
	}
	if strings.Contains(result.Error, "private storage failure detail") {
		t.Fatal("raw storage error leaked")
	}
}

func TestNativeTrackerApprovalPrivacyMetadataAndPersistence(t *testing.T) {
	server, count := trackerFixture(t)
	root := t.TempDir()
	files := filepath.Join(root, "files")
	native, err := newNativeTorrents(root, files, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer native.Close()
	gid := addTrackerFixtureTorrent(t, native, false)
	before := native.session.GetTorrent(gid).InfoHash()
	url := server.URL + "/announce"
	if _, err = native.addTrackers(gid, []string{url}, false); err == nil {
		t.Fatal("unapproved tracker change accepted")
	}
	if _, err = native.addTrackers(gid, []string{url, "file:///invalid"}, true); err == nil {
		t.Fatal("invalid batch accepted")
	}
	state, err := native.trackers(gid)
	if err != nil || !state.CanAdd || len(state.URLs) != 0 || count.Load() != 0 {
		t.Fatal("rejected input caused a side effect")
	}
	changed, err := native.addTrackers(gid, []string{url, url}, true)
	if err != nil || len(changed.Added) != 1 || len(changed.Skipped) != 1 {
		t.Fatal("tracker addition/deduplication failed", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for count.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if count.Load() == 0 {
		t.Fatal("real local tracker was not contacted")
	}
	if native.session.GetTorrent(gid).InfoHash() != before {
		t.Fatal("tracker change rewrote the torrent info hash")
	}
	again, err := native.addTrackers(gid, []string{url}, true)
	if err != nil || len(again.Added) != 0 || len(again.Skipped) != 1 {
		t.Fatal("duplicate tracker was persisted twice")
	}
	private := addTrackerFixtureTorrent(t, native, true)
	state, err = native.trackers(private)
	if err != nil || !state.Private || state.CanAdd {
		t.Fatal("private torrent is not protected", err)
	}
	if _, err = native.addTrackers(private, []string{server.URL + "/must-not-add"}, true); err == nil {
		t.Fatal("private torrent changed")
	}
	state, _ = native.trackers(private)
	if len(state.URLs) != 0 {
		t.Fatal("private tracker list was modified")
	}
	raw, err := native.addURI("magnet:?xt=urn:btih:" + strings.Repeat("1", 40))
	if err != nil {
		t.Fatal(err)
	}
	var magnet string
	_ = json.Unmarshal(raw, &magnet)
	state, err = native.trackers(magnet)
	if err != nil || state.MetadataReady || state.CanAdd {
		t.Fatal("unknown metadata treated as public")
	}
	if _, err = native.addTrackers(magnet, []string{url}, true); err == nil {
		t.Fatal("tracker added before metadata was verified")
	}
	if _, _, err = native.control("pause", gid); err != nil {
		t.Fatal(err)
	}
	if err = native.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := newNativeTorrents(root, files, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	state, err = restored.trackers(gid)
	if err != nil || len(state.URLs) != 1 || state.URLs[0] != url || !state.CanAdd {
		t.Fatal("paused tracker list did not survive restart", err)
	}
	if restored.session.GetTorrent(gid).InfoHash() != before {
		t.Fatal("restart changed the info hash")
	}
}

func TestNativeTrackerConcurrentDedupAndRemoval(t *testing.T) {
	root := t.TempDir()
	native, err := newNativeTorrents(root, filepath.Join(root, "files"), "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer native.Close()
	gid := addTrackerFixtureTorrent(t, native, false)
	server, _ := trackerFixture(t)
	var added atomic.Int32
	var workers sync.WaitGroup
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			out, err := native.addTrackers(gid, []string{server.URL + "/announce"}, true)
			if err != nil {
				t.Error(err)
				return
			}
			added.Add(int32(len(out.Added)))
		}()
	}
	workers.Wait()
	if added.Load() != 1 {
		t.Fatal("concurrent requests appended duplicate trackers")
	}
	workers.Add(2)
	go func() { defer workers.Done(); _, _, _ = native.control("remove", gid) }()
	go func() { defer workers.Done(); _, _ = native.addTrackers(gid, []string{server.URL + "/other"}, true) }()
	workers.Wait()
	if _, err = native.trackers(gid); err == nil {
		t.Fatal("removed torrent still editable")
	}
}

func TestTorrentTrackerAPIAuthorizationAndValidation(t *testing.T) {
	root := t.TempDir()
	native, err := newNativeTorrents(root, filepath.Join(root, "files"), "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer native.Close()
	gid := addTrackerFixtureTorrent(t, native, false)
	app := &App{engine: &Engine{Native: native}, token: strings.Repeat("a", 64), host: "127.0.0.1:8431"}
	handler := app.handler()
	call := func(method, body, authorization, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://"+app.host+"/api/torrent-trackers?gid="+gid, strings.NewReader(body))
		r.Header.Set("Authorization", authorization)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if call("GET", "", "", "").Code != 401 {
		t.Fatal("unauthenticated API request accepted")
	}
	if call("GET", "", "Bearer "+strings.Repeat("b", 64), "").Code != 401 {
		t.Fatal("different capability token accepted")
	}
	if call("GET", "", "Bearer "+app.token, "https://example.test").Code != 403 {
		t.Fatal("website origin accepted")
	}
	if call("GET", "", "Bearer "+app.token, "").Code != 200 {
		t.Fatal("authorized tracker read rejected")
	}
	for _, body := range []string{
		`{"gid":"` + gid + `","urls":["http://127.0.0.1:1/announce"]}`,
		`{"gid":"` + gid + `","urls":[],"approved":true}`,
		`{"gid":"` + gid + `","urls":["file:///bad"],"approved":true}`,
		`{"gid":"` + gid + `","urls":["http://127.0.0.1:1/announce"],"approved":true,"unknown":true}`,
		`{} {}`,
	} {
		if call("POST", body, "Bearer "+app.token, "http://"+app.host).Code != 400 {
			t.Fatal("invalid or unapproved tracker request accepted")
		}
	}
	server, _ := trackerFixture(t)
	body, _ := json.Marshal(map[string]any{"gid": gid, "urls": []string{server.URL + "/announce"}, "approved": true})
	if call("POST", string(body), "Bearer "+app.token, "http://"+app.host).Code != 200 {
		t.Fatal("approved tracker addition rejected")
	}
}
