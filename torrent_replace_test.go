package main

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"
	"time"

	rain "github.com/cenkalti/rain/v2/torrent"
)

func pauseTrackerFixture(t *testing.T, n *NativeTorrents, gid string) {
	t.Helper()
	tor := n.session.GetTorrent(gid)
	if err := tor.Stop(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for tor.Stats().Status != rain.Stopped {
		if time.Now().After(deadline) {
			t.Fatal("torrent did not stop")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestNativeTrackerReplacementAtomicReviewAndPersistence(t *testing.T) {
	root := ownedTestDir(t)
	files := filepath.Join(root, "files")
	n, err := newNativeTorrents(root, files, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	first, firstCount := trackerFixture(t)
	second, secondCount := trackerFixture(t)
	gid := addTrackerFixtureTorrent(t, n, false)
	if _, err = n.addTrackers(gid, []string{first.URL + "/announce"}, true); err != nil {
		t.Fatal(err)
	}
	pauseTrackerFixture(t, n, gid)
	hash := n.session.GetTorrent(gid).InfoHash()
	current, err := n.trackers(gid)
	if err != nil || !current.CanReplace {
		t.Fatal("paused replacement unavailable", err)
	}
	next := []string{second.URL + "/announce"}
	for _, tc := range []struct {
		expected, urls []string
		approved       bool
	}{
		{current.URLs, next, false}, {[]string{}, next, true}, {current.URLs, []string{next[0], next[0]}, true}, {current.URLs, []string{next[0], "file:///private"}, true}, {nil, next, true}, {current.URLs, nil, true},
	} {
		if _, err := n.replaceTrackers(gid, tc.expected, tc.urls, tc.approved); err == nil {
			t.Fatal("invalid or stale replacement accepted")
		}
		unchanged, _ := n.trackers(gid)
		if !slices.Equal(unchanged.URLs, current.URLs) {
			t.Fatal("rejected replacement changed list")
		}
	}
	oldCount := firstCount.Load()
	result, err := n.replaceTrackers(gid, current.URLs, next, true)
	if err != nil || !slices.Equal(result.URLs, next) {
		t.Fatal("replacement failed", err)
	}
	time.Sleep(30 * time.Millisecond)
	if secondCount.Load() != 0 || firstCount.Load() != oldCount {
		t.Fatal("paused replacement contacted tracker")
	}
	if n.session.GetTorrent(gid).InfoHash() != hash {
		t.Fatal("replacement changed info hash")
	}
	if err = n.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := newNativeTorrents(root, files, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	result, err = restored.trackers(gid)
	if err != nil || !slices.Equal(result.URLs, next) || !result.CanReplace {
		t.Fatal("replacement not restored", err)
	}
	tor := restored.session.GetTorrent(gid)
	if err = tor.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for secondCount.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("replacement tracker never announced")
		}
		time.Sleep(time.Millisecond)
	}
	if firstCount.Load() != oldCount {
		t.Fatal("removed tracker announced after restart")
	}
	pauseTrackerFixture(t, restored, gid)
	result, err = restored.replaceTrackers(gid, next, []string{}, true)
	if err != nil || len(result.URLs) != 0 {
		t.Fatal("remove all trackers failed", err)
	}
}

func TestNativeTrackerReplacementPrivateUnknownAndRunningRefused(t *testing.T) {
	root := ownedTestDir(t)
	n, err := newNativeTorrents(root, filepath.Join(root, "files"), "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	private := addTrackerFixtureTorrent(t, n, true)
	pauseTrackerFixture(t, n, private)
	active := addTrackerFixtureTorrent(t, n, false)
	raw, err := n.addURI("magnet:?xt=urn:btih:0123456789012345678901234567890123456789")
	if err != nil {
		t.Fatal(err)
	}
	var unknown string
	if json.Unmarshal(raw, &unknown) != nil {
		t.Fatal("invalid magnet ID")
	}
	pauseTrackerFixture(t, n, unknown)
	for _, gid := range []string{private, active, unknown, "missing"} {
		if _, err = n.replaceTrackers(gid, []string{}, []string{}, true); err == nil {
			t.Fatal("unsafe torrent replacement accepted")
		}
	}
}
