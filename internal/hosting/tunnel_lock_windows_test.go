//go:build windows

package hosting

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTunnelWritersSerializeAndRespectCancellation(t *testing.T) {
	account, tunnel := id(), id()
	started := make(chan error, 1)
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		unlock, e := acquireTunnelLock(context.Background(), account, tunnel)
		started <- e
		if e == nil {
			<-release
			unlock()
		}
	}()
	if e := <-started; e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	unlock, e := acquireTunnelLock(ctx, account, tunnel)
	cancel()
	if e == nil {
		unlock()
	}
	close(release)
	<-done
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("Concurrent writer acquired the tunnel lock", e)
	}
	unlock, e = acquireTunnelLock(context.Background(), account, tunnel)
	if e != nil {
		t.Fatal("Released tunnel lock remained blocked", e)
	}
	unlock()
}
