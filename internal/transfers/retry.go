package transfers

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type transientError struct {
	message string
	after   time.Duration
}

func (e *transientError) Error() string { return e.message }

func connectionError(err error) error {
	message := "HTTP connection failed; check the address, TLS certificate and network"
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	var wrapped *url.Error
	if errors.As(err, &wrapped) {
		err = wrapped.Err
	}
	var certificate *tls.CertificateVerificationError
	var record tls.RecordHeaderError
	var unknown x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var invalid x509.CertificateInvalidError
	var dns *net.DNSError
	if errors.As(err, &dns) && dns.IsNotFound {
		return errors.New(message)
	}
	// Certificate and redirect-policy failures require a user decision, not retries.
	if errors.As(err, &certificate) || errors.As(err, &record) || errors.As(err, &unknown) || errors.As(err, &hostname) || errors.As(err, &invalid) {
		return errors.New(message)
	}
	var network net.Error
	var operation *net.OpError
	if errors.As(err, &network) && (network.Timeout() || network.Temporary()) || errors.As(err, &operation) && (operation.Op == "dial" || operation.Op == "read" || operation.Op == "write") || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return &transientError{message: message}
	}
	return errors.New(message)
}

func bodyError(err error, message string) error {
	classified := connectionError(err)
	var temporary *transientError
	if errors.As(classified, &temporary) {
		temporary.message = message
		return temporary
	}
	return errors.New(message)
}

func responseError(response *http.Response) error {
	message := fmt.Sprintf("HTTP server returned status %d", response.StatusCode)
	switch response.StatusCode {
	case 408, 429, 500, 502, 503, 504:
		after := time.Duration(0)
		value := strings.TrimSpace(response.Header.Get("Retry-After"))
		if value != "" && strings.TrimLeft(value, "0123456789") == "" {
			seconds, err := strconv.ParseUint(value, 10, 64)
			if err != nil || seconds > 900 {
				return errors.New(message + "; server requests a longer wait, retry manually later")
			}
			after = time.Duration(seconds) * time.Second
		} else if date, err := http.ParseTime(value); err == nil {
			after = max(0, time.Until(date))
			if after > 15*time.Minute {
				return errors.New(message + "; server requests a longer wait, retry manually later")
			}
		}
		return &transientError{message: message, after: after}
	}
	return errors.New(message)
}

func validRetry(job Job) bool {
	if job.RetryLimit < 0 || job.RetryLimit > 5 || job.RetryCount < 0 || job.RetryCount > job.RetryLimit || job.RetryAt < 0 || job.RetryAt > 253402300799000 {
		return false
	}
	if job.Status == "retrying" {
		return job.RetryAt > 0 && job.RetryCount > 0 && job.RetryAt <= time.Now().Add(24*time.Hour).UnixMilli()
	}
	return job.RetryAt == 0
}

// Caller holds m.mu. A retry releases its queue slot; the scheduler cannot
// execute it until the intent, count and due time have been saved successfully.
func (m *Manager) retry(job *Job, err error) bool {
	var temporary *transientError
	if !errors.As(err, &temporary) || job.RetryCount >= job.RetryLimit {
		return false
	}
	delay := m.retryBase * time.Duration(1<<job.RetryCount)
	delay += time.Duration(rand.Int64N(int64(delay/4) + 1))
	delay = max(delay, temporary.after)
	job.RetryCount++
	job.RetryAt = time.Now().Add(delay).UnixMilli()
	job.Status = "retrying"
	job.Error = temporary.message
	if m.save() != nil {
		job.Status = "error"
		job.RetryAt = 0
		job.Error = "Cannot persist automatic retry; retry manually after checking app data folder access"
	}
	return true
}
