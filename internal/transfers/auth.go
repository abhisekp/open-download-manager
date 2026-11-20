package transfers

import (
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func validCredentialRef(ref string) bool {
	if len(ref) != 32 || ref != strings.ToLower(ref) {
		return false
	}
	_, err := hex.DecodeString(ref)
	return err == nil
}
func credentialURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.User == nil && u.Hostname() != ""
}
func requestOrigin(u *url.URL) string {
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	port := u.Port()
	if port != "" {
		if n, err := strconv.Atoi(port); err == nil {
			port = strconv.Itoa(n)
		}
	}
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return u.Scheme + "://" + host + ":" + port
}

func (m *Manager) authorize(request *http.Request, job Job) error {
	if job.CredentialRef == "" {
		return nil
	}
	if request.URL.Scheme != "https" || m.lookupCredential == nil {
		return errors.New("saved login unavailable; check Settings and resume")
	}
	user, password, err := m.lookupCredential(request.Context(), job.CredentialRef, job.URL)
	if err != nil {
		return errors.New("saved login unavailable, locked or outside its website; check Settings and resume")
	}
	if request.Context().Err() != nil {
		return request.Context().Err()
	}
	request.SetBasicAuth(user, password)
	return nil
}

// SetCredential never races an active writer and never reuses a partial file
// under a different account. Changing account can change the served content.
func (m *Manager) SetCredential(id, ref string) error {
	m.mu.Lock()
	job := m.jobs[id]
	if job == nil {
		m.mu.Unlock()
		return errors.New("unknown HTTP download")
	}
	raw := job.URL
	m.mu.Unlock()
	if ref != "" {
		if !validCredentialRef(ref) || !credentialURL(raw) || m.validateCredential == nil || m.lookupCredential == nil {
			return errors.New("saved login unavailable")
		}
		if m.validateCredential(ref, raw) != nil {
			return errors.New("saved login does not match this HTTPS website")
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	job = m.jobs[id]
	if m.closed || job == nil || m.running[id] != nil || job.Status != "paused" && job.Status != "error" && job.Status != "waiting" && job.Status != "scheduled" {
		return errors.New("pause this download and wait for its requests to stop first")
	}
	if job.CredentialRef == ref {
		return nil
	}
	previous := copyJob(job)
	job.CredentialRef = ref
	job.ETag = ""
	job.Modified = ""
	job.Segments = nil
	job.Completed = 0
	job.Committed = 0
	job.Total = 0
	job.StartAt = 0
	job.Status = "paused"
	job.Error = ""
	if err := m.save(); err != nil {
		*job = previous
		return errors.New("cannot save changed login reference")
	}
	return nil
}
