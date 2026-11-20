package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

type TorrentLimits struct {
	DownloadKiB int64 `json:"downloadKiB"`
	UploadKiB   int64 `json:"uploadKiB"`
}
type torrentLimitInput struct {
	DownloadKiB *int64 `json:"downloadKiB"`
	UploadKiB   *int64 `json:"uploadKiB"`
}

func (v torrentLimitInput) value() (TorrentLimits, bool) {
	if v.DownloadKiB == nil || v.UploadKiB == nil {
		return TorrentLimits{}, false
	}
	out := TorrentLimits{DownloadKiB: *v.DownloadKiB, UploadKiB: *v.UploadKiB}
	return out, out.valid()
}

func (v TorrentLimits) valid() bool {
	return v.DownloadKiB >= 0 && v.DownloadKiB <= 1<<20 && v.UploadKiB >= 0 && v.UploadKiB <= 1<<20
}

type torrentLimitFile struct {
	Version int           `json:"version"`
	Limits  TorrentLimits `json:"limits"`
}

func loadTorrentLimits(file string) (TorrentLimits, error) {
	var state struct {
		Version int               `json:"version"`
		Limits  torrentLimitInput `json:"limits"`
	}
	info, err := os.Lstat(file)
	if os.IsNotExist(err) {
		return TorrentLimits{}, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1024 {
		return TorrentLimits{}, errors.New("torrent limit settings require recovery; original retained")
	}
	f, err := os.Open(file)
	if err != nil {
		return TorrentLimits{}, errors.New("torrent limit settings unavailable")
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) || opened.Size() > 1024 {
		return TorrentLimits{}, errors.New("torrent limit settings changed while opening; original retained")
	}
	d := json.NewDecoder(io.LimitReader(f, 1025))
	d.DisallowUnknownFields()
	if d.Decode(&state) != nil || d.Decode(new(any)) != io.EOF || state.Version != 1 {
		return TorrentLimits{}, errors.New("invalid torrent limit settings; original retained")
	}
	value, valid := state.Limits.value()
	if !valid {
		return TorrentLimits{}, errors.New("invalid torrent limit settings; original retained")
	}
	return value, nil
}
func writeTorrentLimits(file string, value TorrentLimits) error {
	data, _ := json.Marshal(torrentLimitFile{Version: 1, Limits: value})
	f, err := os.CreateTemp(filepath.Dir(file), ".torrent-limits-*")
	if err != nil {
		return errors.New("cannot save torrent limits; previous settings remain active")
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil || os.Rename(f.Name(), file) != nil {
		return errors.New("cannot save torrent limits; previous settings remain active")
	}
	return nil
}
func (n *NativeTorrents) getLimits() (TorrentLimits, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed || n.session == nil {
		return TorrentLimits{}, errors.New("native torrent engine unavailable")
	}
	return n.limits, nil
}
func (n *NativeTorrents) setLimits(value TorrentLimits) error {
	if !value.valid() {
		return errors.New("use whole KiB/s values between 0 and 1048576")
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed || n.session == nil {
		return errors.New("native torrent engine unavailable")
	}
	if err := writeTorrentLimits(n.limitFile, value); err != nil {
		return err
	}
	// Both values were validated before persistence and the session is owned
	// under the lifecycle lock. The engine setter performs no fallible I/O.
	if err := n.session.SetSpeedLimits(value.DownloadKiB, value.UploadKiB); err != nil {
		return err
	}
	n.limits = value
	return nil
}
func (a *App) torrentLimitsAPI(w http.ResponseWriter, r *http.Request) {
	if a.engine == nil || a.engine.Native == nil {
		http.Error(w, "Native torrent limits unavailable; other downloads still work", 503)
		return
	}
	if r.Method == http.MethodPost {
		var input torrentLimitInput
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
		d.DisallowUnknownFields()
		if d.Decode(&input) != nil || d.Decode(new(any)) != io.EOF {
			http.Error(w, "invalid torrent limits", 400)
			return
		}
		value, valid := input.value()
		if !valid {
			http.Error(w, "provide both torrent limits as supported whole KiB/s values", 400)
			return
		}
		if err := a.engine.Native.setLimits(value); err != nil {
			http.Error(w, err.Error(), 503)
			return
		}
	} else if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", 405)
		return
	}
	value, err := a.engine.Native.getLimits()
	if err != nil {
		http.Error(w, err.Error(), 503)
		return
	}
	respond(w, value)
}
