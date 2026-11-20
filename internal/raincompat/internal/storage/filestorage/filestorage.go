// Package filestorage implements Storage interface that uses files on disk as storage.
package filestorage

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/cenkalti/rain/v2/storage"
)

// FileStorage implements Storage interface for saving files on disk.
type FileStorage struct {
	dest     string
	perm     fs.FileMode
	ownsDest bool
}

// New returns a new FileStorage at the destination.
// If ownsDest is true, dest contains only this torrent and Remove deletes it entirely.
// Otherwise, Remove deletes only the top level entries of the torrent's files.
func New(dest string, perm fs.FileMode, ownsDest bool) (*FileStorage, error) {
	var err error
	dest, err = filepath.Abs(dest)
	if err != nil {
		return nil, err
	}
	return &FileStorage{dest: dest, perm: perm, ownsDest: ownsDest}, nil
}

var (
	_ storage.Storage = (*FileStorage)(nil)
	_ storage.Remover = (*FileStorage)(nil)
)

// Open creates only new owned files and resumes compatible regular files.
// All path operations are relative to an OS-enforced storage root.
func (s *FileStorage) Open(name string, size int64) (f storage.File, exists bool, err error) {
	name = filepath.Clean(name)
	if !isInsideDest(name) || size < 0 {
		return nil, false, fmt.Errorf("invalid torrent storage path or size")
	}
	root, err := s.openRoot()
	if err != nil {
		return nil, false, err
	}
	defer root.Close()
	// Reject existing link components. Root operations independently prevent
	// escape if a component changes while it is being resolved.
	parent := filepath.Dir(name)
	if parent != "." {
		prefix := ""
		for _, part := range strings.Split(parent, string(filepath.Separator)) {
			prefix = filepath.Join(prefix, part)
			info, statErr := root.Lstat(prefix)
			if os.IsNotExist(statErr) {
				if statErr = root.Mkdir(prefix, s.perm); statErr != nil && !os.IsExist(statErr) {
					return nil, false, statErr
				}
				info, statErr = root.Lstat(prefix)
			}
			if statErr != nil {
				return nil, false, statErr
			}
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return nil, false, fmt.Errorf("%w: torrent folder contains a link or non-directory; original retained", storage.ErrUnsafeDestination)
			}
		}
	}
	before, statErr := root.Lstat(name)
	if statErr != nil && !os.IsNotExist(statErr) {
		return nil, false, statErr
	}
	exists = statErr == nil
	if exists && (!before.Mode().IsRegular() || before.Size() != size) {
		return nil, true, fmt.Errorf("%w: existing torrent file conflicts with expected size or type; original retained", storage.ErrUnsafeDestination)
	}
	flags := applyNoAtimeFlag(os.O_RDWR | os.O_SYNC)
	if !exists {
		flags |= os.O_CREATE | os.O_EXCL
	}
	of, err := root.OpenFile(name, flags, s.perm&^0111)
	if err != nil {
		return nil, exists, err
	}
	success := false
	defer func() {
		if !success {
			of.Close()
		}
	}()
	after, err := of.Stat()
	if err != nil {
		return nil, exists, err
	}
	if !after.Mode().IsRegular() || (exists && !os.SameFile(before, after)) {
		return nil, exists, fmt.Errorf("%w: torrent file changed while opening; original retained", storage.ErrUnsafeDestination)
	}
	single, err := hasSingleLink(of)
	if err != nil {
		return nil, exists, err
	}
	if !single {
		return nil, exists, fmt.Errorf("%w: hard-linked torrent files cannot be written; original retained", storage.ErrUnsafeDestination)
	}
	if !exists {
		if err = of.Truncate(size); err != nil {
			return nil, false, err
		}
	}
	if err = disableReadAhead(of); err != nil {
		return nil, exists, err
	}
	success = true
	return of, exists, nil
}

func (s *FileStorage) openRoot() (*os.Root, error) {
	if !s.ownsDest {
		if err := os.MkdirAll(s.dest, s.perm); err != nil {
			return nil, err
		}
		return os.OpenRoot(s.dest)
	}
	// The configured download directory is trusted. A per-torrent child must
	// remain inside it, even when its directory name already exists as a link.
	parent := filepath.Dir(s.dest)
	if err := os.MkdirAll(parent, s.perm); err != nil {
		return nil, err
	}
	anchor, err := os.OpenRoot(parent)
	if err != nil {
		return nil, err
	}
	defer anchor.Close()
	name := filepath.Base(s.dest)
	info, err := anchor.Lstat(name)
	if os.IsNotExist(err) {
		if err = anchor.Mkdir(name, s.perm); err != nil && !os.IsExist(err) {
			return nil, err
		}
		info, err = anchor.Lstat(name)
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("%w: torrent root is a link or non-directory; original retained", storage.ErrUnsafeDestination)
	}
	return anchor.OpenRoot(name)
}

// isInsideDest reports whether the relative path names something strictly under dest.
func isInsideDest(path string) bool {
	return filepath.IsLocal(path) && filepath.Clean(path) != "."
}

// RootDir is the root of opened storage file.
func (s *FileStorage) RootDir() string {
	return s.dest
}

// Remove deletes the torrent's files from disk.
func (s *FileStorage) Remove(m *storage.Manifest, keepData bool) error {
	if keepData {
		return nil
	}
	if s.ownsDest {
		return os.RemoveAll(s.dest)
	}
	if m == nil {
		return nil
	}
	seen := make(map[string]struct{})
	for _, f := range m.Files {
		if f.Padding {
			continue
		}
		top, _, _ := strings.Cut(filepath.ToSlash(f.Path), "/")
		if _, ok := seen[top]; ok {
			continue
		}
		seen[top] = struct{}{}
		if !isInsideDest(top) {
			return fmt.Errorf("file path is not inside the storage root: %q", f.Path)
		}
		if err := os.RemoveAll(filepath.Join(s.dest, top)); err != nil {
			return err
		}
	}
	return nil
}
