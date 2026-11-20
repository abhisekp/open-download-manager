package torrent

import (
	"encoding/json"
	"errors"
	"slices"

	"github.com/cenkalti/rain/v2/internal/resumer/boltdbresumer"
	"github.com/cenkalti/rain/v2/internal/tracker"
	"go.etcd.io/bbolt"
)

// ReplaceTrackers atomically replaces a paused public torrent's tracker list.
// expected is the exact flattened, deduplicated list the owner reviewed.
// No tracker transport is contacted until a later explicit Start.
func (t *Torrent) ReplaceTrackers(expected, urls []string) error {
	if len(expected) > 128 || len(urls) > 128 {
		return errors.New("tracker list exceeds 128 URLs")
	}
	expected = slices.Clone(expected)
	urls = slices.Clone(urls)
	result := make(chan error, 1)
	if !t.torrent.sendCommand(func() { result <- t.replaceTrackers(expected, urls) }) {
		return errClosed
	}
	return <-result
}

func (t *Torrent) replaceTrackers(expected, urls []string) error {
	tor := t.torrent
	if tor.status() != Stopped {
		return errors.New("pause the torrent and wait for it to stop before replacing trackers")
	}
	if tor.info == nil || tor.info.NumPieces == 0 || tor.info.Private {
		return errors.New("tracker replacement requires public, nonempty verified metadata")
	}
	current := []string{}
	seen := map[string]bool{}
	for _, tier := range tor.getTieredTrackers() {
		for _, value := range tier {
			if !seen[value] {
				seen[value] = true
				current = append(current, value)
			}
		}
	}
	if !slices.Equal(expected, current) {
		return errors.New("tracker list changed; refresh and review it again")
	}
	next := make([]tracker.Tracker, 0, len(urls))
	raw := make([][]string, 0, len(urls))
	seen = map[string]bool{}
	for _, value := range urls {
		if seen[value] {
			return errors.New("replacement tracker list contains duplicates")
		}
		seen[value] = true
		tr, err := tor.session.trackerManager.Get(value, tor.session.config.TrackerHTTPTimeout, tor.session.getTrackerUserAgent(false), int64(tor.session.config.TrackerHTTPMaxResponseSize))
		if err != nil {
			return errors.New("replacement tracker URL is unsupported")
		}
		next = append(next, tr)
		raw = append(raw, []string{value})
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	err = tor.session.db.Update(func(tx *bbolt.Tx) error {
		root := tx.Bucket(torrentsBucket)
		if root == nil {
			return errors.New("torrent storage unavailable")
		}
		bucket := root.Bucket([]byte(tor.id))
		if bucket == nil {
			return errors.New("torrent not found")
		}
		return bucket.Put(boltdbresumer.Keys.Trackers, encoded)
	})
	if err != nil {
		return errors.New("cannot save tracker replacement; previous list remains active")
	}
	// Paused state has no announcers or peers. Assignment cannot fail after the
	// transaction commits, and the command barrier excludes concurrent Start.
	tor.trackers = next
	tor.rawTrackers = raw
	return nil
}
