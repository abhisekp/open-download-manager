package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	rain "github.com/cenkalti/rain/v2/torrent"
)

func completedSeedFixture(t *testing.T, n *NativeTorrents) (*rain.Torrent, []byte, string) {
	t.Helper()
	payload := bytes.Repeat([]byte("original bounded-seeding fixture\n"), 4000)
	metadata, hash := fixtureTorrent(t, "shared.bin", payload)
	tor, err := n.session.AddTorrent(bytes.NewReader(metadata), &rain.AddTorrentOptions{ID: "3333333333333333", Stopped: true, StopAfterDownload: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(tor.Dir(), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(tor.Dir(), "shared.bin"), payload, 0600); err != nil {
		t.Fatal(err)
	}
	if err = tor.Start(); err != nil {
		t.Fatal(err)
	}
	waitNative(t, tor)
	pauseTrackerFixture(t, n, tor.ID())
	return tor, payload, hash
}

func waitSeeding(t *testing.T, tor *rain.Torrent) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for tor.Stats().Status != rain.Seeding {
		if time.Now().After(deadline) {
			t.Fatal("approved torrent did not seed")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestBoundedSeedingRealPeerAndDeadline(t *testing.T) {
	root := ownedTestDir(t)
	n, err := newNativeTorrents(root, filepath.Join(root, "files"), "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	tor, payload, hash := completedSeedFixture(t, n)
	if err = tor.SetSeedingDeadline(time.Now().Add(4 * time.Second)); err != nil {
		t.Fatal(err)
	}
	waitSeeding(t, tor)
	var row struct{ Status string }
	if json.Unmarshal(n.row(tor), &row) != nil || row.Status != "seeding" {
		t.Fatal("sharing hidden as completed download")
	}
	targetRoot := ownedTestDir(t)
	target, err := newNativeTorrents(targetRoot, filepath.Join(targetRoot, "files"), "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	raw, err := target.addURI("magnet:?xt=urn:btih:" + hash + "&x.pe=" + url.QueryEscape("127.0.0.1:"+strconv.Itoa(tor.Stats().Port)))
	if err != nil {
		t.Fatal(err)
	}
	var gid string
	json.Unmarshal(raw, &gid)
	download := target.session.GetTorrent(gid)
	waitNative(t, download)
	actual, err := os.ReadFile(filepath.Join(download.Dir(), "shared.bin"))
	if err != nil || !bytes.Equal(payload, actual) {
		t.Fatal("shared payload integrity", err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for tor.Stats().Status != rain.Stopped {
		if time.Now().After(deadline) {
			t.Fatal("expired seeding continued")
		}
		time.Sleep(10 * time.Millisecond)
	}
	state := tor.SeedingState()
	if state.Active || !state.Ready || state.Until > time.Now().Unix() {
		t.Fatal("expired seed state incorrect")
	}
}

func TestSeedingOwnerConsentAndRestart(t *testing.T) {
	root := ownedTestDir(t)
	files := filepath.Join(root, "files")
	n, err := newNativeTorrents(root, files, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	tor, _, _ := completedSeedFixture(t, n)
	gid := tor.ID()
	app := &App{engine: &Engine{Native: n}, token: "seed-owner", host: "127.0.0.1:9944"}
	for _, tc := range []struct {
		body, token, origin string
		code                int
	}{
		{`{"minutes":10}`, "", "", 401},
		{`{"minutes":10}`, app.token, "https://other.invalid", 403},
		{`{"minutes":10}`, app.token, "", 400},
		{`{"minutes":1441,"approved":true}`, app.token, "", 400},
		{`{"minutes":-1}`, app.token, "", 400},
		{`{"minutes":1,"ratioPercent":-1,"approved":true}`, app.token, "", 400},
		{`{"minutes":1,"ratioPercent":10001,"approved":true}`, app.token, "", 400},
		{`{"minutes":0,"ratioPercent":100,"approved":true}`, app.token, "", 400},
		{`{"minutes":1,"ratioPercent":0.5,"approved":true}`, app.token, "", 400},
		{`{"minutes":1,"ratioPercent":100,"approved":false}`, app.token, "", 400},
		{`{"minutes":1,"approved":true,"hidden":true}`, app.token, "", 400},
	} {
		body := strings.Replace(tc.body, "{", `{"gid":"`+gid+`",`, 1)
		req := httptest.NewRequest("POST", "http://"+app.host+"/api/torrent-seeding", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tc.token)
		req.Header.Set("Origin", tc.origin)
		out := httptest.NewRecorder()
		app.handler().ServeHTTP(out, req)
		if out.Code != tc.code {
			t.Fatalf("seeding API status %d want %d", out.Code, tc.code)
		}
	}
	before := tor.SeedingState()
	if before.Active || before.Until != 0 {
		t.Fatal("refused request changed seeding")
	}
	state, err := n.seeding(gid, 1, true, true)
	if err != nil || state.Until <= time.Now().Unix() {
		t.Fatal("approved seeding failed", err)
	}
	waitSeeding(t, tor)
	if err = n.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := newNativeTorrents(root, files, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	resumed := restored.session.GetTorrent(gid)
	waitSeeding(t, resumed)
	if resumed.SeedingState().Until != state.Until {
		t.Fatal("restart extended seeding consent")
	}
	if _, err = restored.seeding(gid, 0, false, true); err != nil {
		t.Fatal(err)
	}
	pauseTrackerFixture(t, restored, gid)
	if resumed.SeedingState().Until != 0 {
		t.Fatal("stop failed to clear deadline")
	}
	active := addTrackerFixtureTorrent(t, restored, false)
	if _, err = restored.seeding(active, 1, true, true); err == nil {
		t.Fatal("incomplete torrent allowed seeding")
	}
}

func TestSeedingRatioRealPeerAndRestart(t *testing.T) {
	root := ownedTestDir(t)
	files := filepath.Join(root, "files")
	n, err := newNativeTorrents(root, files, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	tor, payload, hash := completedSeedFixture(t, n)
	app := &App{engine: &Engine{Native: n}, token: "ratio-fixture-owner", host: "127.0.0.1:9945"}
	req := httptest.NewRequest("POST", "http://"+app.host+"/api/torrent-seeding", strings.NewReader(`{"gid":"`+tor.ID()+`","minutes":5,"ratioPercent":100,"approved":true}`))
	req.Header.Set("Authorization", "Bearer "+app.token)
	out := httptest.NewRecorder()
	app.handler().ServeHTTP(out, req)
	if out.Code != 200 {
		t.Fatal("approved ratio API failed", out.Code, out.Body.String())
	}
	var policy rain.SeedingState
	if err = json.Unmarshal(out.Body.Bytes(), &policy); err != nil || policy.RatioPercent != 100 || policy.TargetBytes != int64(len(payload)) {
		t.Fatal("wrong reviewed ratio", err, policy)
	}
	waitSeeding(t, tor)
	targetRoot := ownedTestDir(t)
	target, err := newNativeTorrents(targetRoot, filepath.Join(targetRoot, "files"), "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	raw, err := target.addURI("magnet:?xt=urn:btih:" + hash + "&x.pe=" + url.QueryEscape("127.0.0.1:"+strconv.Itoa(tor.Stats().Port)))
	if err != nil {
		t.Fatal(err)
	}
	var gid string
	if err = json.Unmarshal(raw, &gid); err != nil {
		t.Fatal(err)
	}
	download := target.session.GetTorrent(gid)
	waitNative(t, download)
	actual, err := os.ReadFile(filepath.Join(download.Dir(), "shared.bin"))
	if err != nil || !bytes.Equal(actual, payload) {
		t.Fatal("ratio transfer corrupted bytes", err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for tor.Stats().Status != rain.Stopped {
		if time.Now().After(deadline) {
			t.Fatal("reached ratio kept sharing")
		}
		time.Sleep(10 * time.Millisecond)
	}
	state := tor.SeedingState()
	if state.Active || state.UploadedBytes < state.TargetBytes || state.Until <= time.Now().Unix() {
		t.Fatal("ratio did not stop before time limit", state)
	}
	if err = n.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := newNativeTorrents(root, files, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	resumed := restored.session.GetTorrent(tor.ID())
	if resumed == nil || resumed.Stats().Status != rain.Stopped || resumed.SeedingState().RatioPercent != 100 {
		t.Fatal("reached ratio resumed or disappeared")
	}
	if _, err = restored.seedingPolicy(tor.ID(), 5, 100, true, true); err == nil {
		t.Fatal("already reached ratio accepted")
	}
	if _, err = restored.seedingPolicy(tor.ID(), 5, 200, true, true); err != nil {
		t.Fatal("newly approved higher goal refused", err)
	}
	if _, err = restored.seedingPolicy(tor.ID(), 0, 0, false, true); err != nil {
		t.Fatal(err)
	}
	state = resumed.SeedingState()
	if state.RatioPercent != 0 || state.TargetBytes != 0 || state.Until != 0 {
		t.Fatal("stop retained sharing policy", state)
	}
}
