//go:build linux || darwin

package filestorage

import (
	"errors"
	"os"
	"syscall"
)

func hasSingleLink(f *os.File) (bool, error) {
	info, err := f.Stat()
	if err != nil {
		return false, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false, errors.New("cannot verify torrent file link count")
	}
	return stat.Nlink == 1, nil
}
