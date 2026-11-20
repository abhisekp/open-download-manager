package torrent

import (
	"errors"
	"math"
	"strconv"
	"time"

	"github.com/cenkalti/rain/v2/internal/resumer/boltdbresumer"
	"go.etcd.io/bbolt"
)

var seedDeadlineKey = []byte("odm_seed_deadline_v1")
var seedRatioKey = []byte("odm_seed_ratio_percent_v1")

type SeedingState struct {
	Until         int64 `json:"until"`
	Ready         bool  `json:"ready"`
	Active        bool  `json:"active"`
	RatioPercent  int   `json:"ratioPercent"`
	TargetBytes   int64 `json:"targetBytes"`
	UploadedBytes int64 `json:"uploadedBytes"`
}

func (t *Torrent) SeedingState() SeedingState {
	return query(t.torrent, func() SeedingState {
		tor := t.torrent
		return SeedingState{Until: tor.seedUntil, Ready: tor.status() == Stopped && tor.info != nil && tor.info.NumPieces > 0 && tor.bitfield != nil && tor.bitfield.All(), Active: tor.status() == Seeding, RatioPercent: tor.seedRatioPercent, TargetBytes: tor.seedRatioTarget, UploadedBytes: tor.bytesUploaded.Count()}
	})
}

// SetSeedingDeadline explicitly starts bounded sharing of a complete paused
// torrent. Zero stops sharing. The persistent absolute deadline is checked
// before restart and by the run-loop ticker.
func (t *Torrent) SetSeedingDeadline(until time.Time) error {
	return t.SetSeedingPolicy(until, 0)
}

// SetSeedingPolicy optionally stops on lifetime uploaded payload divided by
// content size. Integer percent avoids floating-point persistence ambiguity.
// This is an accounting target, not a strict traffic quota: in-flight payloads,
// ticker granularity and crash checkpoint gaps can overshoot it.
func (t *Torrent) SetSeedingPolicy(until time.Time, ratioPercent int) error {
	if ratioPercent < 0 || ratioPercent > 10000 || (until.IsZero() && ratioPercent != 0) {
		return errors.New("choose an upload ratio from 0.01 to 100, or zero for time only")
	}
	seconds := int64(0)
	if !until.IsZero() {
		if !until.After(time.Now()) || until.After(time.Now().Add(24*time.Hour)) {
			return errors.New("choose a future seeding deadline within 24 hours")
		}
		seconds = until.Unix()
	}
	result := make(chan error, 1)
	if !t.torrent.sendCommand(func() {
		tor := t.torrent
		if seconds > 0 && (tor.status() != Stopped || tor.info == nil || tor.info.NumPieces == 0 || tor.bitfield == nil || !tor.bitfield.All()) {
			result <- errors.New("seeding requires a fully paused, verified complete torrent")
			return
		}
		var target int64
		if ratioPercent > 0 {
			var err error
			target, err = seedingRatioTarget(tor.info.Length-tor.info.Padding, ratioPercent)
			if err != nil {
				result <- err
				return
			}
			if tor.bytesUploaded.Count() >= target {
				result <- errors.New("this torrent has already reached the chosen lifetime upload ratio")
				return
			}
		}
		err := tor.session.db.Update(func(tx *bbolt.Tx) error {
			root := tx.Bucket(torrentsBucket)
			if root == nil {
				return errors.New("torrent storage unavailable")
			}
			b := root.Bucket([]byte(tor.id))
			if b == nil {
				return errors.New("torrent not found")
			}
			if err := b.Put(seedDeadlineKey, []byte(strconv.FormatInt(seconds, 10))); err != nil {
				return err
			}
			if err := b.Put(seedRatioKey, []byte(strconv.Itoa(ratioPercent))); err != nil {
				return err
			}
			if err := b.Put(boltdbresumer.Keys.StopAfterDownload, []byte(strconv.FormatBool(seconds == 0))); err != nil {
				return err
			}
			return b.Put(boltdbresumer.Keys.Started, []byte(strconv.FormatBool(seconds > 0)))
		})
		if err != nil {
			if seconds == 0 {
				tor.seedUntil = 0
				tor.seedRatioPercent, tor.seedRatioTarget = 0, 0
				tor.stopAfterDownload = true
				tor.stop(nil)
				result <- errors.New("sharing stopped for this session, but saving failed; the previous deadline may resume after restart")
				return
			}
			result <- errors.New("cannot save seeding choice; previous choice remains active")
			return
		}
		tor.seedUntil = seconds
		tor.seedRatioPercent, tor.seedRatioTarget = ratioPercent, target
		tor.stopAfterDownload = seconds == 0
		if seconds == 0 {
			tor.stop(nil)
		} else {
			tor.start()
		}
		result <- nil
	}) {
		return errClosed
	}
	return <-result
}

func (s *Session) readSeedingPolicy(id string) (int64, int, error) {
	var value int64
	var ratio int
	err := s.db.View(func(tx *bbolt.Tx) error {
		root := tx.Bucket(torrentsBucket)
		if root == nil {
			return errors.New("torrent storage unavailable")
		}
		b := root.Bucket([]byte(id))
		if b == nil {
			return errors.New("torrent not found")
		}
		raw := b.Get(seedDeadlineKey)
		rawRatio := b.Get(seedRatioKey)
		if len(rawRatio) > 0 {
			var err error
			if len(rawRatio) > 5 {
				return errors.New("invalid seeding ratio; retained")
			}
			ratio, err = strconv.Atoi(string(rawRatio))
			if err != nil || ratio < 0 || ratio > 10000 {
				return errors.New("invalid seeding ratio; retained")
			}
		}
		if len(raw) == 0 {
			if ratio != 0 {
				return errors.New("seeding ratio has no sharing deadline; retained")
			}
			return nil
		}
		if len(raw) > 20 {
			return errors.New("invalid seeding deadline; retained")
		}
		var err error
		value, err = strconv.ParseInt(string(raw), 10, 64)
		if err != nil || value < 0 || value > 253402300799 {
			return errors.New("invalid seeding deadline; retained")
		}
		if value == 0 && ratio != 0 {
			return errors.New("seeding ratio has no sharing deadline; retained")
		}
		return nil
	})
	return value, ratio, err
}

func seedingRatioTarget(size int64, percent int) (int64, error) {
	if size <= 0 || percent < 1 || percent > 10000 {
		return 0, errors.New("invalid ratio or torrent content size")
	}
	// ceil(size * percent / 100), without overflowing the intermediate product.
	extra := (size%100*int64(percent) + 99) / 100
	if size/100 > (math.MaxInt64-extra)/int64(percent) {
		return 0, errors.New("upload ratio target is too large")
	}
	return size/100*int64(percent) + extra, nil
}

func (t *torrent) seedingFinished(now time.Time) bool {
	return t.seedUntil > 0 && (now.Unix() >= t.seedUntil || (t.seedRatioTarget > 0 && t.bytesUploaded.Count() >= t.seedRatioTarget))
}

func (t *torrent) enforceSeedingDeadline(now time.Time) {
	if !t.seedingFinished(now) {
		return
	}
	t.stopAfterDownload = true
	if t.status() == Stopped || t.status() == Stopping {
		return
	}
	// Stop even if storage is unavailable. The expired persistent deadline also
	// prevents automatic sharing after a subsequent restart.
	if err := t.session.resumer.WriteStarted(t.id, false); err != nil {
		t.log.Errorln("cannot save expired seeding state")
	}
	t.stop(nil)
}
