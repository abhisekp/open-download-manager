package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/abhisekp/open-download-manager/internal/transfers"
)

type Engine struct {
	URL, Secret   string
	Client        *http.Client
	Native        *NativeTorrents
	HTTP          *transfers.Manager
	HTTPDirectory string
	rpcOffline    atomic.Bool
}

func (e *Engine) ariaAvailable() bool { return e.URL != "" && !e.rpcOffline.Load() }

func (e *Engine) Call(ctx context.Context, method string, params ...any) (json.RawMessage, error) {
	if method == "addUri" && len(params) > 0 && e.Native == nil {
		if urls, ok := params[0].([]string); ok && len(urls) == 1 && torrentLink(urls[0]) {
			return nil, errors.New("native torrent engine unavailable; preserve its database before recovery")
		}
	}
	if handled, raw, err := e.callHTTP(method, params); handled {
		return raw, err
	}

	if e.Native != nil {
		if method == "addUri" && len(params) > 0 {
			if urls, ok := params[0].([]string); ok && len(urls) == 1 && torrentLink(urls[0]) {
				return e.Native.addURI(urls[0])
			}
		}
		if len(params) > 0 {
			if gid, ok := params[0].(string); ok {
				if handled, result, err := e.Native.control(method, gid); handled {
					return result, err
				}
			}
		}
	}
	if e.URL == "" || e.rpcOffline.Load() && method != "getVersion" {
		switch method {
		case "tellActive", "tellWaiting", "tellStopped":
			return e.mergeNative(method, json.RawMessage(`[]`)), nil
		case "getGlobalStat":
			return e.mergeNative(method, json.RawMessage(`{"downloadSpeed":"0","uploadSpeed":"0"}`)), nil
		case "saveSession", "shutdown":
			return json.RawMessage(`"OK"`), nil
		default:
			return nil, errors.New("install aria2 for non-torrent transfers")
		}
	}
	p := append([]any{"token:" + e.Secret}, params...)
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": "odm", "method": "aria2." + method, "params": p})
	req, err := http.NewRequestWithContext(ctx, "POST", e.URL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := e.Client.Do(req)
	if err != nil {
		e.rpcOffline.Store(true)
		if fallback, ok := e.localStatus(method); ok {
			return fallback, nil
		}
		return nil, errors.New("transfer engine unavailable")
	}
	defer res.Body.Close()
	var out struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err = json.NewDecoder(http.MaxBytesReader(nil, res.Body, 4<<20)).Decode(&out); err != nil {
		e.rpcOffline.Store(true)
		if fallback, ok := e.localStatus(method); ok {
			return fallback, nil
		}
		return nil, err
	}
	e.rpcOffline.Store(false)
	if out.Error != nil {
		return nil, fmt.Errorf("engine error (%d): %s", out.Error.Code, out.Error.Message)
	}
	return e.mergeNative(method, out.Result), nil
}

func (e *Engine) localStatus(method string) (json.RawMessage, bool) {
	switch method {
	case "tellActive", "tellWaiting", "tellStopped":
		return e.mergeNative(method, json.RawMessage(`[]`)), true
	case "getGlobalStat":
		return e.mergeNative(method, json.RawMessage(`{"downloadSpeed":"0","uploadSpeed":"0"}`)), true
	}
	return nil, false
}
func (e *Engine) mergeNative(method string, raw json.RawMessage) json.RawMessage {
	raw = e.mergeHTTP(method, raw)
	if e.Native == nil {
		return raw
	}
	switch method {
	case "tellActive", "tellWaiting", "tellStopped":
		var rows []json.RawMessage
		_ = json.Unmarshal(raw, &rows)
		rows = append(rows, e.Native.rows(method)...)
		if rows == nil {
			rows = []json.RawMessage{}
		}
		raw, _ = json.Marshal(rows)
	case "getGlobalStat":
		var stats map[string]any
		_ = json.Unmarshal(raw, &stats)
		down, up := int64(0), int64(0)
		for _, t := range e.Native.session.ListTorrents() {
			s := t.Stats()
			down += int64(s.Speed.Download)
			up += int64(s.Speed.Upload)
		}
		for key, extra := range map[string]int64{"downloadSpeed": down, "uploadSpeed": up} {
			old, _ := strconv.ParseInt(fmt.Sprint(stats[key]), 10, 64)
			stats[key] = strconv.FormatInt(old+extra, 10)
		}
		raw, _ = json.Marshal(stats)
	}
	return raw
}
func newEngine(endpoint, secret string) *Engine {
	return &Engine{URL: endpoint, Secret: secret, Client: &http.Client{Timeout: 10 * time.Second}}
}
func validateURI(raw string) error {
	if len(raw) > 8192 {
		return errors.New("link is too long")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("invalid link")
	}
	if u.User != nil {
		return errors.New("credential-bearing links are not supported")
	}
	if u.Scheme == "magnet" {
		if strings.HasPrefix(u.Query().Get("xt"), "urn:btih:") {
			return nil
		}
		return errors.New("magnet link needs a BitTorrent info hash")
	}
	if (u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "ftp" && u.Scheme != "sftp") || u.Hostname() == "" {
		return errors.New("use an HTTP, HTTPS, FTP, SFTP or magnet link")
	}
	return nil
}
