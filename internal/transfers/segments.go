package transfers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type Segment struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
	Done  int64 `json:"done"`
}

var errRestartStream = errors.New("range resource changed or range support unavailable")

func copyJob(job *Job) Job {
	copy := *job
	copy.Segments = append([]Segment(nil), job.Segments...)
	return copy
}

func makeSegments(total int64) []Segment {
	rows := make([]Segment, 3)
	width := total / 3
	for i := range rows {
		rows[i] = Segment{Start: width * int64(i), End: width*int64(i+1) - 1}
	}
	rows[2].End = total - 1
	return rows
}

func validSegments(job Job) bool {
	if len(job.Segments) == 0 {
		return true
	}
	if job.SingleStream || len(job.Segments) != 3 || job.Total < 8<<20 || strongETag(job.ETag) == "" || job.Completed > job.Total {
		return false
	}
	next, done := int64(0), int64(0)
	for _, s := range job.Segments {
		if s.Start != next || s.End < s.Start || s.End >= job.Total || s.Done < 0 || s.Done > s.End-s.Start+1 {
			return false
		}
		next = s.End + 1
		done += s.Done
	}
	return next == job.Total && done == job.Committed
}

func (m *Manager) resetSegments(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	job := m.jobs[id]
	old := copyJob(job)
	job.Segments = nil
	job.ETag = ""
	job.Modified = ""
	job.Completed = 0
	job.Committed = 0
	job.Total = 0
	if err := m.save(); err != nil {
		*job = old
		return errors.New("cannot reset changed segment metadata")
	}
	return nil
}

func (m *Manager) segmented(parent context.Context, id string) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	m.mu.Lock()
	job := copyJob(m.jobs[id])
	m.mu.Unlock()
	if !validSegments(job) {
		return errors.New("invalid segment layout")
	}
	file, err := m.root.OpenFile(partial(id), os.O_RDWR, 0600)
	if err != nil {
		return errors.New("cannot open segmented partial file")
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return errors.New("cannot inspect segmented partial file")
	}
	entry, err := m.root.Lstat(partial(id))
	if err != nil || !entry.Mode().IsRegular() || !os.SameFile(entry, info) {
		return errors.New("segmented partial download must be an owned regular file")
	}
	for _, s := range job.Segments {
		if s.Done > 0 && s.Start+s.Done > info.Size() {
			return errors.New("segment checkpoint exceeds saved file; remove and retry this transfer")
		}
	}
	// Discard a crash tail beyond the declared resource, never trusting its size
	// as proof that any hole or range has been downloaded.
	if info.Size() > job.Total {
		if file.Truncate(job.Total) != nil {
			return errors.New("cannot discard segmented crash tail")
		}
	}
	var progressMu, syncMu sync.Mutex
	written := make([]int64, len(job.Segments))
	verified := make([]bool, len(job.Segments))
	for i, s := range job.Segments {
		written[i] = s.Done
		verified[i] = s.Done == s.End-s.Start+1
	}
	checkpoint := func() error {
		syncMu.Lock()
		defer syncMu.Unlock()
		progressMu.Lock()
		target := append([]int64(nil), written...)
		for i, s := range job.Segments {
			// Full coverage becomes durable only after the response ended cleanly.
			// An overlong/malformed body must not finalize on a later resume.
			if target[i] == s.End-s.Start+1 && !verified[i] {
				target[i]--
			}
		}
		progressMu.Unlock()
		if file.Sync() != nil {
			return errors.New("cannot flush segment checkpoint")
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		current := m.jobs[id]
		if current == nil {
			return context.Canceled
		}
		current.Committed = 0
		for i, n := range target {
			current.Segments[i].Done = n
			current.Committed += n
		}
		if m.save() != nil {
			return errors.New("cannot save segment checkpoint")
		}
		return nil
	}
	var workers sync.WaitGroup
	errorsC := make(chan error, len(job.Segments))
	m.mu.Lock()
	m.jobs[id].Connections = 0
	m.jobs[id].Completed = job.Committed
	for _, s := range job.Segments {
		if s.Done < s.End-s.Start+1 {
			m.jobs[id].Connections++
		}
	}
	m.mu.Unlock()
	started, window := time.Now(), int64(0)
	for i, s := range job.Segments {
		if s.Done == s.End-s.Start+1 {
			continue
		}
		workers.Add(1)
		go func(i int, s Segment) {
			defer func() {
				m.mu.Lock()
				if current := m.jobs[id]; current != nil && current.Connections > 0 {
					current.Connections--
				}
				m.mu.Unlock()
				workers.Done()
			}()
			workerErr := m.segmentBody(ctx, job, s, file, func(n int) {
				progressMu.Lock()
				written[i] += int64(n)
				total := int64(0)
				for _, value := range written {
					total += value
				}
				window += int64(n)
				m.mu.Lock()
				m.jobs[id].Completed = total
				if elapsed := time.Since(started); elapsed >= 500*time.Millisecond {
					m.jobs[id].Speed = int64(float64(window) / elapsed.Seconds())
					started = time.Now()
					window = 0
				}
				m.mu.Unlock()
				progressMu.Unlock()
			}, checkpoint)
			if workerErr != nil {
				errorsC <- workerErr
				cancel()
			} else {
				progressMu.Lock()
				verified[i] = true
				progressMu.Unlock()
			}
		}(i, s)
	}
	workers.Wait()
	close(errorsC)
	var first error
	for failure := range errorsC {
		if errors.Is(failure, errRestartStream) {
			first = errRestartStream
			break
		}
		if first == nil || errors.Is(first, context.Canceled) {
			first = failure
		}
	}
	// Save only offsets whose preceding writes have been synced, even on pause.
	if err = checkpoint(); err != nil {
		return err
	}
	if parent.Err() != nil {
		return parent.Err()
	}
	if first != nil {
		return first
	}
	if err = file.Close(); err != nil {
		closed = true
		return errors.New("cannot close completed segment file")
	}
	closed = true
	return m.finalize(parent, id, job.Total)
}

func (m *Manager) segmentBody(parent context.Context, job Job, s Segment, file *os.File, progress func(int), checkpoint func() error) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	start := s.Start + s.Done
	request, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, job.URL, nil)
	if requestErr != nil {
		return errors.New("invalid segmented HTTP request")
	}
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, s.End))
	request.Header.Set("If-Range", job.ETag)
	response, err := m.client.Do(request)
	if err != nil {
		if parent.Err() != nil {
			return parent.Err()
		}
		return errors.New("segmented HTTP connection failed; resume to retry")
	}
	defer response.Body.Close()
	if response.StatusCode == 200 || response.StatusCode == 412 || response.StatusCode == 416 {
		return errRestartStream
	}
	if response.StatusCode != 206 {
		return fmt.Errorf("segment server returned status %d", response.StatusCode)
	}
	if response.Header.Get("ETag") != job.ETag {
		return errRestartStream
	}
	if response.Header.Get("Content-Range") != fmt.Sprintf("bytes %d-%d/%d", start, s.End, job.Total) || response.ContentLength >= 0 && response.ContentLength != s.End-start+1 {
		return errors.New("invalid segmented HTTP byte range")
	}
	if encoding := response.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		return errors.New("unsupported segment content encoding")
	}
	var lastRead atomic.Int64
	lastRead.Store(time.Now().UnixNano())
	watchDone := make(chan struct{})
	defer close(watchDone)
	go func() {
		ticker := time.NewTicker(min(5*time.Second, m.stallTimeout/2))
		defer ticker.Stop()
		for {
			select {
			case <-watchDone:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				if time.Since(time.Unix(0, lastRead.Load())) > m.stallTimeout {
					cancel()
					return
				}
			}
		}
	}()
	buffer := make([]byte, 32768)
	offset := start
	lastCheckpoint := time.Now()
	for {
		if ctx.Err() != nil {
			if parent.Err() != nil {
				return parent.Err()
			}
			return errors.New("HTTP segment stalled; resume to retry")
		}
		m.mu.Lock()
		limiter := m.limiter
		m.mu.Unlock()
		size := max(1, min(len(buffer), limiter.Burst()))
		n, readErr := response.Body.Read(buffer[:size])
		if n > 0 {
			lastRead.Store(time.Now().UnixNano())
			if int64(n) > s.End-offset+1 {
				return errors.New("HTTP segment exceeded its declared range")
			}
			if err = limiter.WaitN(ctx, n); err != nil {
				return err
			}
			count, writeErr := file.WriteAt(buffer[:n], offset)
			if writeErr != nil || count != n {
				return errors.New("cannot write HTTP segment; check free space and folder access")
			}
			offset += int64(n)
			progress(n)
			if time.Since(lastCheckpoint) >= time.Second {
				if err = checkpoint(); err != nil {
					return err
				}
				lastCheckpoint = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			if parent.Err() != nil {
				return parent.Err()
			}
			return errors.New("HTTP segment interrupted; resume to retry")
		}
	}
	if offset != s.End+1 {
		return errors.New("HTTP segment body length mismatch")
	}
	return nil
}
