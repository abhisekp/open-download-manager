package torrent

import (
	"errors"
	"strconv"
	"time"

	"github.com/cenkalti/rain/v2/internal/resumer/boltdbresumer"
	"go.etcd.io/bbolt"
)

var seedDeadlineKey = []byte("odm_seed_deadline_v1")

type SeedingState struct {
	Until  int64 `json:"until"`
	Ready  bool  `json:"ready"`
	Active bool  `json:"active"`
}

func (t *Torrent) SeedingState() SeedingState {
	return query(t.torrent, func() SeedingState {
		tor := t.torrent
		return SeedingState{Until: tor.seedUntil, Ready: tor.status() == Stopped && tor.info != nil && tor.info.NumPieces > 0 && tor.bitfield != nil && tor.bitfield.All(), Active: tor.status() == Seeding}
	})
}

// SetSeedingDeadline explicitly starts bounded sharing of a complete paused
// torrent. Zero stops sharing. The persistent absolute deadline is checked
// before restart and by the run-loop ticker.
func (t *Torrent) SetSeedingDeadline(until time.Time) error {
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
			if err := b.Put(boltdbresumer.Keys.StopAfterDownload, []byte(strconv.FormatBool(seconds == 0))); err != nil {
				return err
			}
			return b.Put(boltdbresumer.Keys.Started, []byte(strconv.FormatBool(seconds > 0)))
		})
		if err != nil {
			if seconds == 0 {
				tor.seedUntil = 0
				tor.stopAfterDownload = true
				tor.stop(nil)
				result <- errors.New("sharing stopped for this session, but saving failed; the previous deadline may resume after restart")
				return
			}
			result <- errors.New("cannot save seeding choice; previous choice remains active")
			return
		}
		tor.seedUntil = seconds
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

func (s *Session) readSeedingDeadline(id string) (int64, error) {
	var value int64
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
		if len(raw) == 0 {
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
		return nil
	})
	return value, err
}

func (t *torrent) enforceSeedingDeadline(now time.Time) {
	if t.seedUntil == 0 || now.Unix() < t.seedUntil {
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
