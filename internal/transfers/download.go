package transfers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

func strongETag(value string) string {
	if len(value) > 512 || !strings.HasPrefix(value, `"`) || !strings.HasSuffix(value, `"`) || strings.Count(value, `"`) != 2 {
		return ""
	}
	return value
}
func modifiedTime(value string) string {
	if _, err := http.ParseTime(value); err != nil {
		return ""
	}
	return value
}

func (m *Manager) download(parent context.Context, id string) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	m.mu.Lock()
	job := *m.jobs[id]
	m.mu.Unlock()
	file, err := m.root.OpenFile(partial(id), os.O_RDWR, 0600)
	if err != nil {
		return errors.New("cannot open partial download")
	}
	closed, ready := false, false
	var offset int64
	defer func() {
		if !closed {
			if ready && file.Sync() == nil {
				m.mu.Lock()
				m.jobs[id].Committed = offset
				_ = m.save()
				m.mu.Unlock()
			}
			_ = file.Close()
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return errors.New("cannot inspect partial download")
	}
	entry, err := m.root.Lstat(partial(id))
	if err != nil || !entry.Mode().IsRegular() || !os.SameFile(entry, info) {
		return errors.New("partial download must be an owned regular file")
	}
	offset = min(info.Size(), job.Committed)
	validator := strongETag(job.ETag)
	if validator == "" {
		validator = modifiedTime(job.Modified)
	}
	if validator == "" || job.Total > 0 && offset >= job.Total {
		offset = 0
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, job.URL, nil)
	if err != nil {
		return errors.New("invalid HTTP request")
	}
	request.Header.Set("Accept-Encoding", "identity")
	if offset > 0 {
		request.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
		request.Header.Set("If-Range", validator)
	}
	response, err := m.client.Do(request)
	if err != nil {
		return errors.New("HTTP connection failed; check the address, TLS certificate and network")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("HTTP server returned status %d", response.StatusCode)
	}
	if encoding := response.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		return errors.New("server returned unsupported content encoding")
	}
	total := response.ContentLength
	if total < 0 {
		total = 0
	}
	if response.StatusCode == http.StatusPartialContent {
		var start, end, size int64
		if _, err = fmt.Sscanf(response.Header.Get("Content-Range"), "bytes %d-%d/%d", &start, &end, &size); err != nil || response.Header.Get("Content-Range") != fmt.Sprintf("bytes %d-%d/%d", start, end, size) || start != offset || end < start || size <= end || size < 0 || response.ContentLength >= 0 && response.ContentLength != end-start+1 {
			return errors.New("invalid resumed HTTP byte range")
		}
		if offset > 0 && ((strongETag(job.ETag) != "" && response.Header.Get("ETag") != job.ETag) || (strongETag(job.ETag) == "" && response.Header.Get("Last-Modified") != job.Modified)) {
			_ = response.Body.Close()
			_ = file.Close()
			closed = true
			m.mu.Lock()
			m.jobs[id].ETag = ""
			m.jobs[id].Modified = ""
			err = m.save()
			m.mu.Unlock()
			if err != nil {
				return errors.New("cannot persist changed resource metadata")
			}
			return m.download(parent, id)
		}
		total = size
	} else {
		offset = 0
	}
	if err = file.Truncate(offset); err != nil {
		return errors.New("cannot reset partial file")
	}
	if _, err = file.Seek(offset, io.SeekStart); err != nil {
		return errors.New("cannot seek partial file")
	}
	m.mu.Lock()
	current := m.jobs[id]
	if _, parameters, e := mime.ParseMediaType(response.Header.Get("Content-Disposition")); e == nil && parameters["filename"] != "" {
		current.Name = safeName(path.Base(strings.ReplaceAll(parameters["filename"], `\`, "/")))
	}
	current.Total = total
	current.Completed = offset
	current.Committed = offset
	current.ETag = strongETag(response.Header.Get("ETag"))
	current.Modified = modifiedTime(response.Header.Get("Last-Modified"))
	err = m.save()
	m.mu.Unlock()
	if err != nil {
		return errors.New("cannot persist download metadata")
	}
	ready = true
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
	started := time.Now()
	checkpoint := started
	window := int64(0)
	for {
		if err = ctx.Err(); err != nil {
			if parent.Err() == nil {
				return errors.New("HTTP transfer stalled; resume to retry")
			}
			return err
		}
		size := len(buffer)
		m.mu.Lock()
		limiter := m.limiter
		m.mu.Unlock()
		burst := limiter.Burst()
		if burst < size {
			size = burst
		}
		if size < 1 {
			size = 1
		}
		n, readErr := response.Body.Read(buffer[:size])
		if n > 0 {
			lastRead.Store(time.Now().UnixNano())
			if total > 0 && int64(n) > total-offset {
				return errors.New("HTTP server exceeded the declared resource size")
			}
			if err = limiter.WaitN(ctx, n); err != nil {
				return err
			}
			written, writeErr := file.Write(buffer[:n])
			if writeErr != nil || written != n {
				return errors.New("cannot write download; check free space and folder access")
			}
			offset += int64(n)
			window += int64(n)
			m.mu.Lock()
			current = m.jobs[id]
			current.Completed = offset
			if elapsed := time.Since(started); elapsed >= 500*time.Millisecond {
				current.Speed = int64(float64(window) / elapsed.Seconds())
				started = time.Now()
				window = 0
			}
			m.mu.Unlock()
			if time.Since(checkpoint) >= time.Second {
				if file.Sync() != nil {
					return errors.New("cannot checkpoint partial download")
				}
				m.mu.Lock()
				m.jobs[id].Committed = offset
				err = m.save()
				m.mu.Unlock()
				if err != nil {
					return errors.New("cannot save transfer checkpoint")
				}
				checkpoint = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			if ctx.Err() != nil && parent.Err() == nil {
				return errors.New("HTTP transfer stalled; resume to retry")
			}
			return errors.New("HTTP transfer interrupted; resume to retry")
		}
	}
	if total > 0 && offset != total {
		return errors.New("HTTP body length does not match the resource size")
	}
	if err = file.Sync(); err != nil {
		return errors.New("cannot flush completed download")
	}
	if err = file.Close(); err != nil {
		return errors.New("cannot close completed download")
	}
	closed = true
	m.mu.Lock()
	m.jobs[id].Committed = offset
	m.mu.Unlock()
	return m.finalize(ctx, id, offset)
}

func (m *Manager) finalize(ctx context.Context, id string, size int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	job := m.jobs[id]
	if ctx.Err() != nil || job.Status != "active" {
		return context.Canceled
	}
	base := job.Name
	for suffix := 0; suffix < 1000; suffix++ {
		name := collisionName(base, suffix)
		// Exclusive reservation avoids overwriting an existing file, including
		// symlinks. Rename replaces only the empty file created by this operation.
		reservation, err := m.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return errors.New("cannot reserve destination filename")
		}
		if err = reservation.Close(); err != nil {
			return errors.New("cannot close destination reservation")
		}
		previous := *job
		job.Name = name
		job.Total = size
		job.Completed = size
		job.Status = "finalizing"
		if err = m.save(); err != nil {
			*job = previous
			_ = m.root.Remove(name)
			return errors.New("cannot journal completed download")
		}
		if err = m.root.Rename(partial(id), name); err != nil {
			job.Status = "error"
			job.Error = "Cannot finalize download; partial file retained"
			_ = m.root.Remove(name)
			_ = m.save()
			return errors.New(job.Error)
		}
		job.Status = "complete"
		job.Speed = 0
		job.Error = ""
		if err = m.save(); err != nil {
			return errors.New("completed file saved; final status will recover on restart")
		}
		return nil
	}
	return errors.New("too many filename collisions; choose another download folder")
}

func collisionName(base string, index int) string {
	if index == 0 {
		return base
	}
	extension := filepath.Ext(base)
	if len(extension) > 32 {
		extension = ""
	}
	stem := strings.TrimSuffix(base, extension)
	suffix := " (" + strconv.Itoa(index) + ")"
	for len(stem)+len(suffix)+len(extension) > 160 {
		_, size := utf8.DecodeLastRuneInString(stem)
		stem = stem[:len(stem)-size]
	}
	if stem == "" {
		stem = "download"
	}
	return stem + suffix + extension
}

func (m *Manager) recoverFinalization(job *Job) {
	job.Status = "error"
	job.Error = "Completed transfer needs review; files retained"
	part, err := m.root.Lstat(partial(job.ID))
	if err == nil {
		if !part.Mode().IsRegular() || part.Size() != job.Committed || job.Committed != job.Completed || job.Total != job.Completed {
			return
		}
		target, e := m.root.Lstat(job.Name)
		if e == nil {
			if !target.Mode().IsRegular() || target.Size() != 0 {
				return
			}
		} else if os.IsNotExist(e) {
			file, e := m.root.OpenFile(job.Name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if e != nil {
				return
			}
			if file.Close() != nil {
				return
			}
		} else {
			return
		}
		if m.root.Rename(partial(job.ID), job.Name) != nil {
			return
		}
	} else if os.IsNotExist(err) {
		info, e := m.root.Lstat(job.Name)
		if e != nil || !info.Mode().IsRegular() || info.Size() != job.Completed {
			return
		}
	} else {
		return
	}
	job.Status = "complete"
	job.Error = ""
}
