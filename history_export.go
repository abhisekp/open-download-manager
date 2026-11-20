package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

type HistoryExportFile struct {
	Name    string   `json:"name"`
	Length  string   `json:"length"`
	Path    string   `json:"path,omitempty"`
	Sources []string `json:"sources,omitempty"`
}
type HistoryExportRecord struct {
	ID         string              `json:"id"`
	Status     string              `json:"status"`
	RecordedAt int64               `json:"recordedAt,omitempty"`
	Files      []HistoryExportFile `json:"files"`
}

func (h *History) export(sensitive bool) ([]byte, int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	records := make([]HistoryExportRecord, 0, len(h.rows))
	keys := make([]string, 0, len(h.rows))
	for id := range h.rows {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		var row struct {
			Status     string `json:"status"`
			RecordedAt int64  `json:"recordedAt"`
			Files      []struct {
				Path   string `json:"path"`
				Length string `json:"length"`
				URIs   []struct {
					URI string `json:"uri"`
				} `json:"uris"`
			} `json:"files"`
		}
		if json.Unmarshal(h.rows[id], &row) != nil {
			return nil, 0, errors.New("history record cannot be exported")
		}
		record := HistoryExportRecord{ID: id, Status: row.Status, RecordedAt: row.RecordedAt, Files: []HistoryExportFile{}}
		for _, f := range row.Files {
			file := HistoryExportFile{Name: path.Base(strings.ReplaceAll(f.Path, `\`, "/")), Length: f.Length}
			if sensitive {
				file.Path = f.Path
				for _, uri := range f.URIs {
					file.Sources = append(file.Sources, uri.URI)
				}
			}
			record.Files = append(record.Files, file)
		}
		records = append(records, record)
	}
	output := struct {
		Version   int                   `json:"version"`
		CreatedAt int64                 `json:"createdAt"`
		Sensitive bool                  `json:"includesSensitiveDetails"`
		Records   []HistoryExportRecord `json:"records"`
	}{1, time.Now().UnixMilli(), sensitive, records}
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil || len(data) > 16<<20 {
		return nil, 0, errors.New("history export exceeds supported size")
	}
	return data, len(records), nil
}

func writeHistoryExport(directory string, data []byte) (string, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return "", errors.New("download folder unavailable for history export")
	}
	defer root.Close()
	random := make([]byte, 12)
	if _, err = rand.Read(random); err != nil {
		return "", errors.New("cannot create history export name")
	}
	name := "odm-history-" + time.Now().UTC().Format("20060102-150405") + "-" + hex.EncodeToString(random) + ".json"
	// Exclusive creation prevents overwriting any existing file or following a
	// leaf symlink. Report success only after bytes have been flushed and closed.
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", errors.New("cannot create a new history export")
	}
	written, writeErr := f.Write(data)
	err = writeErr
	if err == nil && written != len(data) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return "", errors.New("history export failed; an incomplete export may remain in the download folder")
	}
	return name, nil
}

func (a *App) historyExportAPI(w http.ResponseWriter, r *http.Request) {
	if a.history == nil {
		http.Error(w, "Saved history unavailable; original history retained", 503)
		return
	}
	if r.Method == http.MethodGet {
		a.history.mu.Lock()
		count := len(a.history.rows)
		a.history.mu.Unlock()
		respond(w, map[string]any{"count": count, "directory": a.downloads})
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", 405)
		return
	}
	var in struct {
		Approved  bool `json:"approved"`
		Sensitive bool `json:"includeSensitiveDetails"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil || d.Decode(new(any)) != io.EOF {
		http.Error(w, "invalid history export request", 400)
		return
	}
	if !in.Approved {
		http.Error(w, "approve saving this history export to the download folder first", 403)
		return
	}
	if !a.historyExportMu.TryLock() {
		http.Error(w, "history export already in progress", 409)
		return
	}
	defer a.historyExportMu.Unlock()
	// A just-completed transfer may precede the periodic history flush. Refresh
	// available engine records before taking the saved-history export snapshot.
	if a.engine != nil {
		observation := a.history.observation()
		if raw, refreshErr := a.engine.Call(r.Context(), "tellStopped", 0, 1000); refreshErr == nil {
			var rows []json.RawMessage
			if json.Unmarshal(raw, &rows) != nil {
				http.Error(w, "current history could not be read safely", 503)
				return
			}
			if _, refreshErr = a.history.mergeObserved(rows, observation); refreshErr != nil {
				if errors.Is(refreshErr, errHistoryObservationStale) {
					http.Error(w, "History changed during export; review and retry", 409)
					return
				}
				http.Error(w, "history could not be saved before export; check app data folder access", 503)
				return
			}
		}
	}
	data, count, err := a.history.export(in.Sensitive)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	name, err := writeHistoryExport(a.downloads, data)
	if err != nil {
		http.Error(w, err.Error(), 503)
		return
	}
	respond(w, map[string]any{"name": name, "records": count, "includesSensitiveDetails": in.Sensitive})
}
