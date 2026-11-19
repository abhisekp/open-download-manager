package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func verifyFile(ctx context.Context, dir, path, expected string) (string, bool, error) {
	want, e := hex.DecodeString(expected)
	if e != nil || len(want) != 32 {
		return "", false, errors.New("enter a 64-character SHA-256 checksum")
	}
	rel, e := filepath.Rel(dir, path)
	if e != nil {
		return "", false, e
	}
	root, e := os.OpenRoot(dir)
	if e != nil {
		return "", false, e
	}
	defer root.Close()
	f, e := root.Open(rel)
	if e != nil {
		return "", false, errors.New("downloaded file is outside the download folder or unavailable")
	}
	defer f.Close()
	before, e := f.Stat()
	if e != nil {
		return "", false, e
	}
	if !before.Mode().IsRegular() {
		return "", false, errors.New("only regular files can be verified")
	}
	if before.Size() > 1<<30 {
		return "", false, errors.New("this synchronous verifier supports files up to 1 GiB; use an external checksum tool for larger files")
	}
	hash := sha256.New()
	buf := make([]byte, 128<<10)
	for {
		if e = ctx.Err(); e != nil {
			return "", false, e
		}
		n, err := f.Read(buf)
		if n > 0 {
			hash.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", false, err
		}
	}
	after, e := f.Stat()
	if e != nil {
		return "", false, e
	}
	if before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return "", false, errors.New("file changed during verification")
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	return digest, strings.EqualFold(digest, expected), nil
}
func (a *App) verify(ctx context.Context, gid, expected string) (map[string]any, error) {
	raw, e := a.engine.Call(ctx, "tellStatus", gid)
	if e != nil {
		raw = a.history.get(gid)
	}
	var row struct {
		Status string
		Files  []struct{ Path string }
	}
	if json.Unmarshal(raw, &row) != nil || row.Status != "complete" || len(row.Files) == 0 {
		return nil, errors.New("select a completed download")
	}
	digest, match, e := verifyFile(ctx, a.downloads, row.Files[0].Path, expected)
	return map[string]any{"sha256": digest, "matches": match}, e
}
