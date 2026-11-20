// Package bandwidth contains ODM's cancellation-aware live payload limiter.
// Original ODM code, MIT licensed; upstream runtime attribution is unchanged.
package bandwidth

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type Limiter struct {
	mu             sync.Mutex
	current        *rate.Limiter
	changed        chan struct{}
	bytesPerSecond int64
}

func New(bytesPerSecond int64) *Limiter {
	l := &Limiter{changed: make(chan struct{})}
	l.set(bytesPerSecond)
	return l
}
func (l *Limiter) set(bytesPerSecond int64) {
	l.bytesPerSecond = max(0, bytesPerSecond)
	limit := rate.Inf
	burst := 65536
	if bytesPerSecond > 0 {
		limit = rate.Limit(bytesPerSecond)
		burst = int(max(16384, min(65536, bytesPerSecond)))
	}
	l.current = rate.NewLimiter(limit, burst)
}

// Set wakes reservations made under the old setting. Existing peers keep this
// shared wrapper, so changing a cap never requires reconnecting the swarm.
func (l *Limiter) Set(bytesPerSecond int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.bytesPerSecond == max(0, bytesPerSecond) {
		return
	}
	l.set(bytesPerSecond)
	close(l.changed)
	l.changed = make(chan struct{})
}

func (l *Limiter) Wait(stop <-chan struct{}, bytes int64) error {
	for bytes > 0 {
		select {
		case <-stop:
			return context.Canceled
		default:
		}
		l.mu.Lock()
		current, changed := l.current, l.changed
		size := min(bytes, int64(current.Burst()))
		now := time.Now()
		reservation := current.ReserveN(now, int(size))
		delay := reservation.DelayFrom(now)
		l.mu.Unlock()
		if delay <= 0 {
			bytes -= size
			continue
		}
		timer := time.NewTimer(delay)
		select {
		case <-stop:
			timer.Stop()
			reservation.Cancel()
			return context.Canceled
		case <-changed:
			timer.Stop()
			reservation.Cancel()
			continue
		case <-timer.C:
			bytes -= size
		}
	}
	return nil
}
