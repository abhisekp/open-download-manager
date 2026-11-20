package main

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"strconv"

	"github.com/abhisekp/open-download-manager/internal/transfers"
)

func (e *Engine) callHTTP(method string, params []any) (bool, json.RawMessage, error) {
	if e.HTTP == nil {
		return false, nil, nil
	}
	if len(params) > 0 {
		if id, ok := params[0].(string); ok {
			if handled, raw, err := e.httpControl(method, id); handled {
				return handled, raw, err
			}
		}
	}
	switch method {
	case "addUri":
		if e.ariaAvailable() || len(params) == 0 {
			return false, nil, nil
		}
		urls, ok := params[0].([]string)
		if !ok || len(urls) != 1 || torrentLink(urls[0]) {
			return false, nil, nil
		}
		parsed, err := url.Parse(urls[0])
		if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" {
			return false, nil, nil
		}
		id, err := e.HTTP.Add(urls[0])
		raw, _ := json.Marshal(id)
		return true, raw, err
	case "changeGlobalOption":
		if len(params) == 0 {
			return false, nil, nil
		}
		options, ok := params[0].(map[string]string)
		if !ok {
			return false, nil, nil
		}
		value, exists := options["max-overall-download-limit"]
		if !exists {
			return false, nil, nil
		}
		limit, err := strconv.ParseUint(value, 10, 64)
		if err == nil {
			err = e.HTTP.SetLimit(limit)
		}
		if err != nil {
			return true, nil, err
		}
		if !e.ariaAvailable() {
			return true, json.RawMessage(`"OK"`), nil
		}
	case "saveSession":
		if err := e.HTTP.Flush(); err != nil {
			return true, nil, err
		}
	case "shutdown":
		if err := e.HTTP.Close(); err != nil {
			return true, nil, err
		}
	}
	return false, nil, nil
}

func (e *Engine) httpRow(job transfers.Job) json.RawMessage {
	status := job.Status
	if status == "retrying" {
		status = "waiting"
	}
	if status == "finalizing" {
		status = "active"
	}
	row, _ := json.Marshal(map[string]any{"gid": job.ID, "status": status, "totalLength": strconv.FormatInt(job.Total, 10), "completedLength": strconv.FormatInt(job.Completed, 10), "downloadSpeed": strconv.FormatInt(job.Speed, 10), "uploadSpeed": "0", "connections": strconv.Itoa(job.Connections), "engine": "native-http", "files": []any{map[string]any{"index": "1", "path": filepath.Join(e.HTTPDirectory, job.Name), "length": strconv.FormatInt(job.Total, 10), "completedLength": strconv.FormatInt(job.Completed, 10), "selected": "true", "uris": []any{map[string]string{"uri": job.URL, "status": "used"}}}}, "errorMessage": job.Error})
	return row
}

func (e *Engine) httpControl(method, id string) (bool, json.RawMessage, error) {
	if e.HTTP == nil || !e.HTTP.Has(id) {
		return false, nil, nil
	}
	if method == "tellStatus" {
		for _, job := range e.HTTP.Jobs() {
			if job.ID == id {
				return true, e.httpRow(job), nil
			}
		}
	}
	err := e.HTTP.Control(id, method)
	raw, _ := json.Marshal(id)
	return true, raw, err
}

func (e *Engine) mergeHTTP(method string, raw json.RawMessage) json.RawMessage {
	if e.HTTP == nil {
		return raw
	}
	switch method {
	case "tellActive", "tellWaiting", "tellStopped":
		var rows []json.RawMessage
		_ = json.Unmarshal(raw, &rows)
		for _, job := range e.HTTP.Jobs() {
			matches := method == "tellActive" && (job.Status == "active" || job.Status == "finalizing") || method == "tellWaiting" && (job.Status == "waiting" || job.Status == "paused" || job.Status == "scheduled" || job.Status == "retrying") || method == "tellStopped" && (job.Status == "complete" || job.Status == "removed" || job.Status == "error")
			if matches {
				rows = append(rows, e.httpRow(job))
			}
		}
		if rows == nil {
			rows = []json.RawMessage{}
		}
		raw, _ = json.Marshal(rows)
	case "getGlobalStat":
		var stats map[string]any
		_ = json.Unmarshal(raw, &stats)
		if stats == nil {
			stats = map[string]any{}
		}
		speed, _ := strconv.ParseInt(asString(stats["downloadSpeed"]), 10, 64)
		for _, job := range e.HTTP.Jobs() {
			speed += job.Speed
		}
		stats["downloadSpeed"] = strconv.FormatInt(speed, 10)
		raw, _ = json.Marshal(stats)
	}
	return raw
}
func asString(value any) string { text, _ := value.(string); return text }
