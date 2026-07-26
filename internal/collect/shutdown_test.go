package collect

import (
	"context"
	"testing"
	"time"
)

type blockingRunner struct{ started chan struct{} }

func (b *blockingRunner) RunContext(ctx context.Context, _ string) (string, error) {
	select {
	case b.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return "", ctx.Err()
}

func (b *blockingRunner) Reset() {}

func (b *blockingRunner) SetPassphrase([]byte) {}

func TestRunWithSinkStopsWhileServerHangs(t *testing.T) {
	// Given a running collector whose only server never answers.
	runner := &blockingRunner{started: make(chan struct{}, 1)}
	collector := newReconnectTestCollector("web", runner)
	collector.cfg.Interval = time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		collector.RunWithSink(ctx, nil)
	}()
	<-runner.started

	// When the context is cancelled mid-poll.
	cancel()

	// Then the loop returns instead of waiting out the poll timeout.
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RunWithSink did not return after cancellation")
	}
}
