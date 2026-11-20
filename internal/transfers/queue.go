package transfers

import (
	"errors"
	"net/url"
	"strings"
)

// QueueSettings limits active native HTTP transfers, including requests waiting
// for a response. Lowering limits does not interrupt already running transfers.
type QueueSettings struct {
	Concurrent int `json:"concurrent"`
	PerHost    int `json:"perHost"`
}

func (q QueueSettings) valid() bool {
	return q.Concurrent >= 1 && q.Concurrent <= 8 && q.PerHost >= 1 && q.PerHost <= 3 && q.PerHost <= q.Concurrent
}

// Host limits intentionally group ports and schemes for a hostname. Redirects
// still use the HTTP transport's per-destination connection limit.
func queueHost(raw string) string {
	u, _ := url.Parse(raw)
	return strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
}

func (m *Manager) QueueSettings() QueueSettings { m.mu.Lock(); defer m.mu.Unlock(); return m.queue }

func (m *Manager) SetQueue(settings QueueSettings) error {
	if !settings.valid() {
		return errors.New("use 1–8 active transfers and 1–3 per host, no greater than the active limit")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("HTTP queue is shutting down")
	}
	previous := m.queue
	m.queue = settings
	if err := m.save(); err != nil {
		m.queue = previous
		return errors.New("cannot save queue settings")
	}
	return nil
}

// Move puts a pending job first or last among pending jobs. Active transfers are
// not interrupted. Every rank change is persisted together or rolled back.
func (m *Manager) Move(id, position string) error {
	if position != "first" && position != "last" {
		return errors.New("choose first or last")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("HTTP queue is shutting down")
	}
	job := m.jobs[id]
	if job == nil || !pending(job.Status) {
		return errors.New("only a pending native HTTP download can be reordered")
	}
	rows := m.snapshot()
	ordered := make([]string, 0, len(rows))
	if position == "first" {
		ordered = append(ordered, id)
	}
	for _, row := range rows {
		if row.ID != id {
			ordered = append(ordered, row.ID)
		}
	}
	if position == "last" {
		ordered = append(ordered, id)
	}
	previous := map[string]int{}
	for i, key := range ordered {
		previous[key] = m.jobs[key].Order
		m.jobs[key].Order = i + 1
	}
	if err := m.save(); err != nil {
		for key, order := range previous {
			m.jobs[key].Order = order
		}
		return errors.New("cannot save queue order")
	}
	return nil
}

func pending(status string) bool {
	return status == "waiting" || status == "scheduled" || status == "paused" || status == "retrying"
}
