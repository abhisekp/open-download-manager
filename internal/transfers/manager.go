// Package transfers provides a bounded native HTTP/HTTPS download queue.
package transfers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/time/rate"
)

type Job struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Total     int64  `json:"total"`
	Completed int64  `json:"completed"`
	Committed int64  `json:"committed"`
	ETag      string `json:"etag"`
	Modified  string `json:"last_modified"`
	Error     string `json:"error"`
	Created   int64  `json:"created"`
	Order     int    `json:"order,omitempty"`
	StartAt   int64  `json:"startAt,omitempty"`
	Speed     int64  `json:"-"`
}

type state struct {
	Version int           `json:"version"`
	Limit   uint64        `json:"limit"`
	Jobs    []Job         `json:"jobs"`
	Queue   QueueSettings `json:"queue,omitempty"`
}
type Config struct {
	DataDir, Downloads string
	Client             *http.Client
	StallTimeout       time.Duration
}
type Manager struct {
	mu            sync.Mutex
	root          *os.Root
	dir, registry string
	jobs          map[string]*Job
	running       map[string]context.CancelFunc
	client        *http.Client
	limiter       *rate.Limiter
	limit         uint64
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	schedulerDone chan struct{}
	closed        bool
	closeDone     chan struct{}
	closeErr      error
	stallTimeout  time.Duration
	queue         QueueSettings
}

func validURL(raw string) bool {
	u, err := url.Parse(raw)
	return len(raw) <= 8192 && err == nil && u.User == nil && u.Hostname() != "" && (u.Scheme == "http" || u.Scheme == "https")
}
func validID(id string) bool {
	if len(id) != 16 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil && strings.ToLower(id) == id
}
func safeName(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`/\\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, value)
	value = strings.Trim(value, " .")
	if value == "" {
		value = "download"
	}
	for len(value) > 160 {
		_, size := utf8.DecodeLastRuneInString(value)
		value = value[:len(value)-size]
	}
	first := strings.ToUpper(strings.SplitN(value, ".", 2)[0])
	if first == "CON" || first == "PRN" || first == "AUX" || first == "NUL" || len(first) == 4 && (strings.HasPrefix(first, "COM") || strings.HasPrefix(first, "LPT")) && first[3] >= '0' && first[3] <= '9' {
		value = "download-" + value
	}
	return value
}
func partial(id string) string { return ".odm-" + id + ".part" }

func Open(config Config) (*Manager, error) {
	if err := os.MkdirAll(config.DataDir, 0700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(config.Downloads, 0700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(config.Downloads)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{root: root, dir: config.Downloads, registry: filepath.Join(config.DataDir, "http-transfers.json"), jobs: map[string]*Job{}, running: map[string]context.CancelFunc{}, limiter: rate.NewLimiter(rate.Inf, 32768), ctx: ctx, cancel: cancel, schedulerDone: make(chan struct{})}
	m.closeDone = make(chan struct{})
	m.queue = QueueSettings{Concurrent: 3, PerHost: 3}
	m.stallTimeout = config.StallTimeout
	if m.stallTimeout <= 0 {
		m.stallTimeout = 30 * time.Second
	}
	fail := func(err error) (*Manager, error) { cancel(); _ = root.Close(); return nil, err }
	if config.Client != nil {
		m.client = config.Client
	} else {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.DisableCompression = true
		transport.ResponseHeaderTimeout = 15 * time.Second
		transport.TLSHandshakeTimeout = 10 * time.Second
		transport.MaxResponseHeaderBytes = 64 << 10
		transport.MaxConnsPerHost = 3
		transport.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
		m.client = &http.Client{Transport: transport}
	}
	// Copy injected test clients so caller-owned state is never mutated.
	client := *m.client
	m.client = &client
	m.client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 10 || !validURL(request.URL.String()) {
			return errors.New("unsupported redirect")
		}
		if len(via) > 0 && via[len(via)-1].URL.Scheme == "https" && request.URL.Scheme != "https" {
			return errors.New("HTTPS downgrade redirect rejected")
		}
		return nil
	}
	file, err := os.Open(m.registry)
	if err == nil {
		defer file.Close()
		var stored state
		decoder := json.NewDecoder(io.LimitReader(file, 8<<20))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&stored) != nil || decoder.Decode(new(any)) != io.EOF || stored.Version != 1 || len(stored.Jobs) > 200 || stored.Limit > 1<<30 {
			return fail(errors.New("invalid native HTTP registry"))
		}
		for _, job := range stored.Jobs {
			if !validID(job.ID) || !validURL(job.URL) || safeName(job.Name) != job.Name || job.Total < 0 || job.Completed < 0 || job.Committed < 0 || job.Committed > job.Completed || job.Order < 0 || job.Order > 200 || job.StartAt < 0 || job.StartAt > 253402300799000 || m.jobs[job.ID] != nil {
				return fail(errors.New("invalid native HTTP task"))
			}
			switch job.Status {
			case "complete", "error", "removed", "paused":
			case "scheduled":
				if job.StartAt == 0 {
					return fail(errors.New("scheduled transfer has no start time"))
				}
			case "active", "waiting":
				job.Status = "paused"
			case "finalizing":
			default:
				return fail(errors.New("unknown native HTTP state"))
			}
			copy := job
			m.jobs[job.ID] = &copy
		}
		m.applyLimit(stored.Limit)
		if stored.Queue != (QueueSettings{}) {
			if !stored.Queue.valid() {
				return fail(errors.New("invalid native HTTP queue settings"))
			}
			m.queue = stored.Queue
		}
		if err = file.Close(); err != nil {
			return fail(err)
		}
	} else if !os.IsNotExist(err) {
		return fail(err)
	}
	for _, job := range m.jobs {
		if job.Status == "finalizing" {
			m.recoverFinalization(job)
		}
	}
	if err = m.save(); err != nil {
		return fail(err)
	}
	go m.schedule()
	return m, nil
}

func (m *Manager) snapshot() []Job {
	rows := make([]Job, 0, len(m.jobs))
	for _, job := range m.jobs {
		rows = append(rows, *job)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Order != rows[j].Order {
			return rows[i].Order < rows[j].Order
		}
		if rows[i].Created != rows[j].Created {
			return rows[i].Created < rows[j].Created
		}
		return rows[i].ID < rows[j].ID
	})
	return rows
}
func (m *Manager) save() error {
	body, err := json.Marshal(state{Version: 1, Limit: m.limit, Jobs: m.snapshot(), Queue: m.queue})
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(m.registry), ".http-state-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(body)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), m.registry)
}
func (m *Manager) applyLimit(limit uint64) {
	m.limit = limit
	burst := 32768
	if limit > 0 && limit < uint64(burst) {
		burst = int(limit)
	}
	value := rate.Inf
	if limit > 0 {
		value = rate.Limit(limit)
	}
	m.limiter = rate.NewLimiter(value, burst)
}
func (m *Manager) SetLimit(limit uint64) error {
	if limit > 1<<30 {
		return errors.New("native HTTP limit must be at most 1 GiB/s")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("HTTP queue is shutting down")
	}
	previous := m.limit
	m.applyLimit(limit)
	if err := m.save(); err != nil {
		m.applyLimit(previous)
		return err
	}
	return nil
}
func (m *Manager) Add(raw string) (string, error) {
	return m.AddPlanned(raw, 0)
}

// AddPlanned persists a one-time schedule before it becomes runnable. A zero
// timestamp queues immediately; a future timestamp is milliseconds since Unix epoch.
func (m *Manager) AddPlanned(raw string, startAt int64) (string, error) {
	if !validURL(raw) {
		return "", errors.New("use an HTTP or HTTPS URL without embedded credentials")
	}
	if startAt != 0 && (startAt <= time.Now().UnixMilli() || startAt > time.Now().AddDate(1, 0, 0).UnixMilli()) {
		return "", errors.New("choose a future start time within one year")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return "", errors.New("HTTP queue is shutting down")
	}
	if len(m.jobs) >= 200 {
		return "", errors.New("native HTTP task limit reached; forget completed tasks")
	}
	key := make([]byte, 8)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	id := hex.EncodeToString(key)
	if m.jobs[id] != nil {
		return "", errors.New("transfer identifier collision; retry")
	}
	u, _ := url.Parse(raw)
	name := safeName(path.Base(u.Path))
	if name == "download-" || name == "_" {
		name = "download"
	}
	file, err := m.root.OpenFile(partial(id), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	if err = file.Close(); err != nil {
		return "", err
	}
	status := "waiting"
	if startAt != 0 {
		status = "scheduled"
	}
	// Keep newly added jobs after the user's existing ordered jobs.
	previousOrders := map[string]int{}
	for i, row := range m.snapshot() {
		previousOrders[row.ID] = row.Order
		m.jobs[row.ID].Order = i + 1
	}
	m.jobs[id] = &Job{ID: id, URL: raw, Name: name, Status: status, Created: time.Now().UnixNano(), Order: len(m.jobs) + 1, StartAt: startAt}
	if err = m.save(); err != nil {
		delete(m.jobs, id)
		for key, order := range previousOrders {
			m.jobs[key].Order = order
		}
		_ = m.root.Remove(partial(id))
		return "", err
	}
	return id, nil
}
func (m *Manager) Has(id string) bool { m.mu.Lock(); defer m.mu.Unlock(); return m.jobs[id] != nil }
func (m *Manager) Jobs() []Job        { m.mu.Lock(); defer m.mu.Unlock(); return m.snapshot() }
func (m *Manager) Control(id, action string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("HTTP queue is shutting down")
	}
	job := m.jobs[id]
	if job == nil {
		return errors.New("unknown transfer")
	}
	previous := *job
	switch action {
	case "pause":
		if job.Status == "complete" || job.Status == "removed" || job.Status == "finalizing" {
			return errors.New("completed transfer cannot be paused")
		}
		job.Status = "paused"
	case "unpause":
		if job.Status == "complete" || job.Status == "removed" || job.Status == "finalizing" {
			return errors.New("completed transfer cannot be resumed")
		}
		job.Status = "waiting"
		job.Error = ""
		job.StartAt = 0
	case "remove":
		if job.Status == "finalizing" {
			return errors.New("transfer is finalizing")
		}
		job.Status = "removed"
	case "removeDownloadResult":
		if m.running[id] != nil && job.Status != "complete" {
			return errors.New("pause or remove the transfer first")
		}
		delete(m.jobs, id)
	default:
		return errors.New("unsupported transfer operation")
	}
	if err := m.save(); err != nil {
		*job = previous
		m.jobs[id] = job
		return err
	}
	if action == "pause" || action == "remove" {
		if cancel := m.running[id]; cancel != nil {
			cancel()
		}
	}
	return nil
}
func (m *Manager) schedule() {
	defer close(m.schedulerDone)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.mu.Lock()
			hosts := map[string]int{}
			for id := range m.running {
				if job := m.jobs[id]; job != nil {
					hosts[queueHost(job.URL)]++
				}
			}
			for _, row := range m.snapshot() {
				if len(m.running) >= m.queue.Concurrent {
					break
				}
				ready := row.Status == "waiting" || row.Status == "scheduled" && row.StartAt <= time.Now().UnixMilli()
				if !ready || m.running[row.ID] != nil || hosts[queueHost(row.URL)] >= m.queue.PerHost {
					continue
				}
				job := m.jobs[row.ID]
				ctx, cancel := context.WithCancel(m.ctx)
				job.Status = "active"
				m.running[row.ID] = cancel
				if err := m.save(); err != nil {
					job.Status = "error"
					job.Error = "Cannot persist transfer state"
					delete(m.running, row.ID)
					cancel()
					continue
				}
				hosts[queueHost(row.URL)]++
				m.wg.Add(1)
				go m.run(ctx, row.ID)
			}
			m.mu.Unlock()
		}
	}
}
func (m *Manager) run(ctx context.Context, id string) {
	defer m.wg.Done()
	err := m.download(ctx, id)
	m.mu.Lock()
	defer m.mu.Unlock()
	job := m.jobs[id]
	if job == nil {
		delete(m.running, id)
		_ = m.save()
		return
	}
	if err != nil && job.Status == "active" {
		if ctx.Err() != nil {
			job.Status = "paused"
		} else {
			job.Status = "error"
			job.Error = err.Error()
		}
	}
	job.Speed = 0
	delete(m.running, id)
	_ = m.save()
}
func (m *Manager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		<-m.closeDone
		return m.closeErr
	}
	m.closed = true
	m.cancel()
	m.mu.Unlock()
	<-m.schedulerDone
	m.wg.Wait()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, job := range m.jobs {
		if job.Status == "waiting" || job.Status == "active" {
			job.Status = "paused"
		}
	}
	err := m.save()
	m.client.CloseIdleConnections()
	closeErr := m.root.Close()
	if err != nil {
		m.closeErr = err
	} else {
		m.closeErr = closeErr
	}
	close(m.closeDone)
	return m.closeErr
}
