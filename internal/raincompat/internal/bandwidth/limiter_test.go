package bandwidth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestSharedPayloadBudget(t *testing.T) {
	l := New(65536)
	stop := make(chan struct{})
	start := time.Now()
	var group sync.WaitGroup
	for range 4 {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := l.Wait(stop, 32768); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	if time.Since(start) < 800*time.Millisecond {
		t.Fatal("each worker received an independent budget")
	}
}
func TestLiveChangeAndCancellationWakeOldReservations(t *testing.T) {
	l := New(1024)
	stop := make(chan struct{})
	done := make(chan error, 1)
	if err := l.Wait(stop, 16384); err != nil {
		t.Fatal(err)
	}
	go func() { done <- l.Wait(stop, 16384) }()
	select {
	case <-done:
		t.Fatal("low cap did not delay payload")
	case <-time.After(30 * time.Millisecond):
	}
	l.Set(0)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("unlimited setting did not wake old wait")
	}
	l.Set(1024)
	if err := l.Wait(stop, 16384); err != nil {
		t.Fatal(err)
	}
	go func() { done <- l.Wait(stop, 16384) }()
	close(stop)
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("cancelled wait returned success")
		}
	case <-time.After(time.Second):
		t.Fatal("cancel failed to wake limiter")
	}
}
