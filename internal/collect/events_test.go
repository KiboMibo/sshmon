package collect

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kibomibo/sshmon/internal/config"
)

func TestSubscribeKeepsLatestEventWithoutBlockingPublisher(t *testing.T) {
	// Given: a slow subscriber with capacity for one event.
	collector := &Collector{}
	events, unsubscribe := collector.Subscribe(1)
	defer unsubscribe()
	first := Event{Snapshot: Snapshot{Time: time.Unix(1, 0)}}
	latest := Event{Snapshot: Snapshot{Time: time.Unix(2, 0)}}

	// When: two events are published before the subscriber reads.
	done := make(chan struct{})
	go func() {
		collector.publish(first)
		collector.publish(latest)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("publisher blocked on slow subscriber")
	}

	// Then: only the newest event remains available.
	select {
	case event := <-events:
		if !event.Snapshot.Time.Equal(latest.Snapshot.Time) {
			t.Fatalf("event time = %s, want %s", event.Snapshot.Time, latest.Snapshot.Time)
		}
	case <-time.After(time.Second):
		t.Fatal("latest event was not delivered")
	}
}

func TestUnsubscribeClosesChannelAndIsIdempotent(t *testing.T) {
	// Given: an active collector subscription.
	collector := &Collector{}
	events, unsubscribe := collector.Subscribe(0)

	// When: it is cancelled twice.
	unsubscribe()
	unsubscribe()

	// Then: its channel is closed.
	if _, open := <-events; open {
		t.Fatal("subscription channel remains open")
	}
}

func TestRunWithSinkPublishesHistoryErrorWithoutStoppingCollector(t *testing.T) {
	// Given: a collector with an optional history sink that fails.
	collector := New(&config.Config{Interval: time.Hour})
	events, unsubscribe := collector.Subscribe(1)
	defer unsubscribe()
	ctx, cancel := context.WithCancel(context.Background())
	wantErr := errors.New("history unavailable")
	sinkCalls := 0

	// When: the initial snapshot reaches the failing sink.
	done := make(chan struct{})
	go func() {
		collector.RunWithSink(ctx, func(context.Context, Snapshot) error {
			sinkCalls++
			cancel()
			return wantErr
		})
		close(done)
	}()

	// Then: the error is published as collector health and the loop exits normally.
	select {
	case event := <-events:
		if event.Snapshot.HistoryErr != wantErr.Error() {
			t.Fatalf("history error = %q, want %q", event.Snapshot.HistoryErr, wantErr)
		}
	case <-time.After(time.Second):
		t.Fatal("history health event was not published")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("collector did not stop after context cancellation")
	}
	if sinkCalls != 1 {
		t.Fatalf("sink calls = %d, want 1", sinkCalls)
	}
}
