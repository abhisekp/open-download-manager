package main

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// v1 extensions are data-only capabilities. No scripts or subprocess permissions.
type Manifest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Capability  string `json:"capability"`
	Endpoint    string `json:"endpoint"`
	Description string `json:"description"`
}

var trackerManifest = Manifest{"org.newtrackon.live", "Live tracker list", "1.0.0", "tracker-feed", "https://newtrackon.com/api/live", "Fetch responding public trackers from newTrackon. Network access requires your approval. Does not modify torrents."}

func validateManifest(m Manifest) error {
	if m.Capability != "tracker-feed" || m.Endpoint != "https://newtrackon.com/api/live" || m.ID != "org.newtrackon.live" {
		return errors.New("unapproved extension capability or endpoint")
	}
	return nil
}
func parseTrackers(r io.Reader) ([]string, error) {
	s := bufio.NewScanner(io.LimitReader(r, 262145))
	seen := map[string]bool{}
	out := []string{}
	size := 0
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		size += len(s.Bytes()) + 1
		if size > 262144 {
			return nil, errors.New("tracker feed exceeds limit")
		}
		if line == "" {
			continue
		}
		u, e := url.Parse(line)
		if e != nil || u.User != nil || u.Hostname() == "" || (u.Scheme != "udp" && u.Scheme != "http" && u.Scheme != "https") {
			return nil, errors.New("invalid tracker feed")
		}
		if !seen[line] {
			out = append(out, line)
			seen[line] = true
		}
		if len(out) > 500 {
			return nil, errors.New("too many trackers")
		}
	}
	if e := s.Err(); e != nil {
		return nil, e
	}
	if len(out) == 0 {
		return nil, errors.New("empty tracker feed")
	}
	return out, nil
}
func fetchTrackers(ctx context.Context, m Manifest) ([]string, error) {
	if e := validateManifest(m); e != nil {
		return nil, e
	}
	client := http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("feed redirects are not allowed") }}
	req, _ := http.NewRequestWithContext(ctx, "GET", m.Endpoint, nil)
	res, e := client.Do(req)
	if e != nil {
		return nil, errors.New("tracker feed unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, errors.New("tracker service returned an error")
	}
	return parseTrackers(res.Body)
}
