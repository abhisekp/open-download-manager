package filestorage

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExistingConflictingFileIsPreserved(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "torrent")
	os.MkdirAll(dest, 0700)
	original := []byte("original unrelated user bytes")
	path := filepath.Join(dest, "conflict.bin")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := New(dest, 0700, true)
	if err != nil {
		t.Fatal(err)
	}
	f, _, err := store.Open("conflict.bin", 2)
	if f != nil {
		f.Close()
	}
	actual, readErr := os.ReadFile(path)
	if err == nil || readErr != nil || !bytes.Equal(actual, original) {
		t.Fatal("existing conflicting file was accepted or changed")
	}
}

func TestStorageCreatesAndResumesOwnedRegularFile(t *testing.T) {
	root := t.TempDir()
	store, err := New(filepath.Join(root, "torrent"), 0700, true)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("original safe partial")
	f, exists, err := store.Open("nested/own.bin", int64(len(payload)))
	if err != nil || exists {
		t.Fatal("create failed", err)
	}
	if _, err = f.WriteAt(payload, 0); err != nil {
		t.Fatal(err)
	}
	f.Close()
	f, exists, err = store.Open("nested/own.bin", int64(len(payload)))
	if err != nil || !exists {
		t.Fatal("resume failed", err)
	}
	defer f.Close()
	actual := make([]byte, len(payload))
	if _, err = f.ReadAt(actual, 0); err != nil || !bytes.Equal(actual, payload) {
		t.Fatal("resume changed bytes", err)
	}
}

func TestStorageRejectsHardLinkedFile(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside.bin")
	payload := []byte("original linked bytes")
	os.WriteFile(outside, payload, 0600)
	dest := filepath.Join(root, "torrent")
	os.MkdirAll(dest, 0700)
	if err := os.Link(outside, filepath.Join(dest, "linked.bin")); err != nil {
		t.Skip("filesystem does not allow fixture hard links:", err)
	}
	store, _ := New(dest, 0700, true)
	f, _, err := store.Open("linked.bin", int64(len(payload)))
	if f != nil {
		f.Close()
	}
	if err == nil {
		t.Fatal("hard-linked target opened for writing")
	}
	actual, _ := os.ReadFile(outside)
	if !bytes.Equal(payload, actual) {
		t.Fatal("external hard-link bytes changed")
	}
}

func TestStorageRejectsSymbolicLinks(t *testing.T) {
	for _, mode := range []string{"file", "directory", "torrent-root"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			outside := filepath.Join(root, "outside")
			os.MkdirAll(outside, 0700)
			payload := []byte("original link destination")
			external := filepath.Join(outside, "original.bin")
			os.WriteFile(external, payload, 0600)
			dest := filepath.Join(root, "downloads", "torrent")
			os.MkdirAll(filepath.Dir(dest), 0700)
			path := "original.bin"
			var err error
			if mode == "torrent-root" {
				err = os.Symlink(outside, dest)
			} else {
				os.MkdirAll(dest, 0700)
				if mode == "file" {
					err = os.Symlink(external, filepath.Join(dest, path))
				} else {
					err = os.Symlink(outside, filepath.Join(dest, "linked"))
					path = "linked/original.bin"
				}
			}
			if err != nil {
				// Directory junctions exercise the Windows reparse-point path
				// without requiring a change to OS developer/trust settings.
				if runtime.GOOS == "windows" && mode != "file" {
					target, link := outside, dest
					if mode == "directory" {
						link = filepath.Join(dest, "linked")
					}
					command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "New-Item -ItemType Junction -Path $env:ODM_TEST_LINK -Target $env:ODM_TEST_TARGET -ErrorAction Stop | Out-Null")
					command.Env = append(os.Environ(), "ODM_TEST_LINK="+link, "ODM_TEST_TARGET="+target)
					if output, junctionErr := command.CombinedOutput(); junctionErr == nil {
						err = nil
						t.Log("using a disposable directory junction fixture")
					} else {
						t.Log("junction fixture unavailable:", junctionErr, string(output))
					}
				}
			}
			if err != nil {
				t.Skip("platform does not allow fixture symlinks:", err)
			}
			store, _ := New(dest, 0700, true)
			f, _, err := store.Open(path, int64(len(payload)))
			if f != nil {
				f.Close()
			}
			if err == nil {
				t.Fatal("symbolic-link destination opened for writing")
			}
			actual, _ := os.ReadFile(external)
			if !bytes.Equal(actual, payload) {
				t.Fatal("symlink destination changed")
			}
		})
	}
}
