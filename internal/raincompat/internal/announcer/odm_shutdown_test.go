package announcer

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/cenkalti/rain/v2/internal/logger"
	"github.com/cenkalti/rain/v2/internal/tracker"
)

type cancelledFixtureTracker struct{}

func (cancelledFixtureTracker) URL() string { return "http://owned-fixture.invalid/announce" }
func (cancelledFixtureTracker) Announce(ctx context.Context, _ tracker.AnnounceRequest) (*tracker.AnnounceResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestCloseWaitsForInFlightTorrentSnapshot(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	closed := make(chan struct{})
	a := NewPeriodicalAnnouncer(cancelledFixtureTracker{}, 1, time.Second, func() tracker.Torrent {
		close(entered)
		<-release
		return tracker.Torrent{}
	}, make(chan struct{}), make(chan []*net.TCPAddr), logger.New("owned-shutdown-fixture"))
	go a.Run()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("snapshot did not start")
	}
	go func() { a.Close(); close(closed) }()
	select {
	case <-a.doneC:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("announcer loop failed to cancel")
	}
	// This is a deliberately blocked snapshot, not a probabilistic load test.
	// Before the fix Close returned here and the owner could free its data.
	select {
	case <-closed:
		close(release)
		t.Fatal("Close returned while torrent snapshot still reading")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("snapshot worker did not finish after release")
	}
}
