package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/zeebo/bencode"
)

type TorrentTrackers struct {
	MetadataReady bool     `json:"metadataReady"`
	Private       bool     `json:"private"`
	CanAdd        bool     `json:"canAdd"`
	Reason        string   `json:"reason,omitempty"`
	URLs          []string `json:"urls"`
}

type TrackerUpdate struct {
	Added     []string `json:"added"`
	Skipped   []string `json:"skipped"`
	Remaining int      `json:"remaining"`
	Error     string   `json:"error,omitempty"`
}

func trackerURL(raw string) (string, error) {
	if raw == "" || len(raw) > 2048 || strings.IndexFunc(raw, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return "", errors.New("tracker URLs must be nonempty, at most 2048 bytes, and contain no whitespace")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "udp") {
		return "", errors.New("use an HTTP, HTTPS or UDP tracker URL without credentials or fragments")
	}
	if port := u.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return "", errors.New("invalid tracker port")
		}
	} else if u.Scheme == "udp" {
		return "", errors.New("UDP trackers require an explicit port")
	}
	return u.String(), nil
}

// Caller owns n.mu. Stats is a run-loop barrier before reading immutable metadata
// and the tracker list; all ODM tracker writes/removals use the same mutex.
func (n *NativeTorrents) trackerState(gid string) (TorrentTrackers, error) {
	out := TorrentTrackers{URLs: []string{}}
	if n.closed {
		return out, errors.New("native torrent engine is closed")
	}
	t := n.session.GetTorrent(gid)
	if t == nil {
		return out, errors.New("native torrent not found")
	}
	stats := t.Stats()
	if stats.Pieces.Total == 0 {
		out.Reason = "Tracker changes require verified, nonempty torrent metadata. Wait for magnet metadata."
		return out, nil
	}
	raw, err := t.Torrent()
	if err != nil {
		return out, errors.New("torrent metadata is unavailable")
	}
	var meta struct {
		Announce string     `bencode:"announce"`
		List     [][]string `bencode:"announce-list"`
		Info     struct {
			Private int `bencode:"private"`
		} `bencode:"info"`
	}
	if bencode.DecodeBytes(raw, &meta) != nil {
		return out, errors.New("torrent metadata could not be read safely")
	}
	out.MetadataReady = true
	out.Private = stats.Private || meta.Info.Private != 0
	out.CanAdd = !out.Private
	if out.Private {
		out.Reason = "Tracker changes are disabled for private torrents to preserve their tracker restrictions."
	}
	seen := map[string]bool{}
	for _, tier := range append([][]string{{meta.Announce}}, meta.List...) {
		for _, value := range tier {
			if value != "" && !seen[value] {
				seen[value] = true
				out.URLs = append(out.URLs, value)
			}
		}
	}
	return out, nil
}

func (n *NativeTorrents) trackers(gid string) (TorrentTrackers, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.trackerState(gid)
}

func (n *NativeTorrents) addTrackers(gid string, input []string, approved bool) (TrackerUpdate, error) {
	out := TrackerUpdate{Added: []string{}, Skipped: []string{}}
	if !approved {
		return out, errors.New("review and approve the chosen trackers before adding them")
	}
	if len(input) < 1 || len(input) > 32 {
		return out, errors.New("choose between 1 and 32 tracker URLs")
	}
	// Validate the entire request before the first persistent/network side effect.
	urls := make([]string, len(input))
	for i, raw := range input {
		value, err := trackerURL(raw)
		if err != nil {
			return out, err
		}
		urls[i] = value
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	state, err := n.trackerState(gid)
	if err != nil {
		return out, err
	}
	if !state.CanAdd {
		return out, errors.New(state.Reason)
	}
	seen := map[string]bool{}
	for _, value := range state.URLs {
		seen[value] = true
	}
	unique := []string{}
	for _, value := range urls {
		if seen[value] {
			out.Skipped = append(out.Skipped, value)
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	if len(state.URLs)+len(unique) > 128 {
		return out, errors.New("torrent tracker limit reached (128)")
	}
	t := n.session.GetTorrent(gid)
	result := persistTrackerBatch(unique, t.AddTracker)
	result.Skipped = out.Skipped
	_ = t.Stats() // wait for the final tracker command before releasing the edit lock
	return result, nil
}

func persistTrackerBatch(urls []string, add func(string) error) TrackerUpdate {
	out := TrackerUpdate{Added: []string{}, Skipped: []string{}}
	for i, value := range urls {
		if err := add(value); err != nil {
			// Rain commits each URL separately. Never claim batch atomicity or
			// pretend earlier persisted additions were rolled back.
			out.Remaining = len(urls) - i
			out.Error = "Some trackers could not be saved. Earlier additions remain; review the current list before retrying."
			return out
		}
		out.Added = append(out.Added, value)
	}
	return out
}

func (a *App) torrentTrackersAPI(w http.ResponseWriter, r *http.Request) {
	if a.engine == nil || a.engine.Native == nil {
		http.Error(w, "native torrents unavailable", 503)
		return
	}
	if r.Method == http.MethodGet {
		state, err := a.engine.Native.trackers(r.URL.Query().Get("gid"))
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		respond(w, state)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var input struct {
		GID      string   `json:"gid"`
		URLs     []string `json:"urls"`
		Approved bool     `json:"approved"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 72<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "invalid tracker request", 400)
		return
	}
	result, err := a.engine.Native.addTrackers(input.GID, input.URLs, input.Approved)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	respond(w, result)
}
