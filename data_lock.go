package main

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/gofrs/flock"
)

const lockProtocol = "OpenDownloadManager OS-lock v2\n"

type dataLock struct {
	lock   *flock.Flock
	legacy string
	once   sync.Once
}

func acquireDataLock(data string) (*dataLock, error) {
	lock := flock.New(filepath.Join(data, ".instance.lock"))
	held, err := lock.TryLock()
	if err != nil {
		return nil, errors.New("cannot lock application data directory")
	}
	if !held {
		_ = lock.Close()
		return nil, errors.New("another ODM instance is using this data directory")
	}
	legacy := filepath.Join(data, "running.lock")
	marker := filepath.Join(legacy, "os-lock-v2")
	failed := func(err error) (*dataLock, error) { _ = lock.Close(); return nil, err }
	if info, err := os.Lstat(legacy); err == nil {
		// The OS lock proves a previous v2 owner is gone. Old releases provide
		// no owner proof; never guess whether their lock is stale or still live.
		if !info.IsDir() {
			return failed(errors.New("invalid legacy lock path"))
		}
		entry, e := os.Lstat(marker)
		if e != nil || !entry.Mode().IsRegular() || entry.Size() != int64(len(lockProtocol)) {
			return failed(errors.New("legacy ODM lock found; stop any old version before manually clearing running.lock"))
		}
		body, e := os.ReadFile(marker)
		if e != nil || string(body) != lockProtocol {
			return failed(errors.New("unrecognized legacy lock owner"))
		}
		if e = os.Remove(marker); e != nil {
			return failed(e)
		}
		if e = os.Remove(legacy); e != nil {
			return failed(errors.New("legacy lock contains unexpected files; review it before restarting"))
		}
	} else if !os.IsNotExist(err) {
		return failed(err)
	}
	// Compatibility marker stops old directory-lock releases from running
	// concurrently. Mkdir never replaces a lock won by an older process.
	if err = os.Mkdir(legacy, 0700); err != nil {
		return failed(errors.New("another or legacy ODM instance owns the data directory"))
	}
	f, err := os.OpenFile(marker, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		_, err = f.WriteString(lockProtocol)
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
	}
	if err != nil {
		_ = os.Remove(marker)
		_ = os.Remove(legacy)
		return failed(err)
	}
	return &dataLock{lock: lock, legacy: legacy}, nil
}

func (l *dataLock) Close() {
	l.once.Do(func() {
		_ = os.Remove(filepath.Join(l.legacy, "os-lock-v2"))
		_ = os.Remove(l.legacy)
		_ = l.lock.Close()
	})
	// The permanent lock file is deliberately retained: unlinking it can let
	// two processes lock different inodes under the same path.
}
