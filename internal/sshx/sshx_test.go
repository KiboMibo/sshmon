package sshx

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func TestRunCommandCancellationDropsConnection(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	var dropped atomic.Bool

	// Given an SSH output operation that remains blocked.
	output := func() ([]byte, error) {
		close(started)
		<-release
		return nil, nil
	}
	done := make(chan error, 1)
	go func() {
		_, err := runCommand(ctx, output, func() { dropped.Store(true) })
		done <- err
	}()
	<-started

	// When its context is cancelled.
	cancel()
	// Then RunContext's shared execution path returns context.Canceled and drops the connection.
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	if !dropped.Load() {
		t.Fatal("connection was not dropped")
	}
}
