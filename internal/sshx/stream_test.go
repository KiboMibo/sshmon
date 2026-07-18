package sshx

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"
)

func TestStreamReaderEmitsLinesAndTerminalError(t *testing.T) {
	t.Parallel()
	// Given a remote stream containing two lines whose session later fails.
	reader, writer := io.Pipe()
	waitErr := errors.New("remote stream failed")
	stream := streamReader(context.Background(), reader, func() error { return waitErr }, func() error { return writer.Close() }, func() {})
	go func() {
		_, _ = io.WriteString(writer, "first\nsecond\n")
		_ = writer.Close()
	}()
	// When the consumer drains the stream.
	var lines []string
	for line := range stream.Lines {
		lines = append(lines, line)
	}
	// Then every line and the terminal session error are delivered once.
	if len(lines) != 2 || lines[0] != "first" || lines[1] != "second" {
		t.Fatalf("lines = %#v", lines)
	}
	if err := <-stream.Errors; !errors.Is(err, waitErr) {
		t.Fatalf("error = %v, want %v", err, waitErr)
	}
	if _, open := <-stream.Errors; open {
		t.Fatal("error channel remained open")
	}
}

func TestStreamReaderCancellationClosesSessionAndChannelsOnce(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	reader, writer := io.Pipe()
	var closes atomic.Int32
	closed := make(chan struct{})
	stream := streamReader(ctx, reader, func() error { <-closed; return nil }, func() error {
		if closes.Add(1) == 1 {
			close(closed)
		}
		return writer.Close()
	}, func() {})
	// Given a running stream with no output.
	// When its context is cancelled and Close is also called repeatedly.
	cancel()
	_ = stream.Close()
	_ = stream.Close()
	// Then channels close and the underlying session closes exactly once.
	select {
	case _, open := <-stream.Lines:
		if open {
			t.Fatal("line channel remained open")
		}
	case <-time.After(time.Second):
		t.Fatal("line channel did not close")
	}
	if closes.Load() != 1 {
		t.Fatalf("session closed %d times", closes.Load())
	}
}
