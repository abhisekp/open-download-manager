package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// Only framework-created directories are accepted. Windows can briefly report
// a removed child as nonempty while its final handles drain; real leaks fail.
func ownedTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Cleanup(func() {
		if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
			t.Error("invalid owned test directory")
			return
		}
		root, err := os.OpenRoot(filepath.Dir(dir))
		if err != nil {
			t.Error(err)
			return
		}
		defer root.Close()
		deadline := time.Now().Add(2 * time.Second)
		for {
			err = root.RemoveAll(filepath.Base(dir))
			if err == nil {
				return
			}
			transient := errors.Is(err, syscall.Errno(145)) || errors.Is(err, syscall.Errno(5)) || errors.Is(err, syscall.Errno(32))
			if runtime.GOOS != "windows" || !transient || time.Now().After(deadline) {
				t.Error("owned test cleanup:", err)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
	return dir
}
