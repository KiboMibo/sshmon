package sshx

import (
	"bufio"
	"context"
	"io"
	"sync"
)

type Stream struct {
	Lines  <-chan string
	Errors <-chan error
	Close  func() error
}

func (c *Client) StreamContext(ctx context.Context, cmd string) (Stream, error) {
	client, err := c.conn()
	if err != nil {
		return Stream{}, err
	}
	session, err := client.NewSession()
	if err != nil {
		c.drop()
		return Stream{}, err
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		_ = session.Close()
		return Stream{}, err
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		_ = session.Close()
		return Stream{}, err
	}
	if err := session.Start(cmd); err != nil {
		_ = session.Close()
		return Stream{}, err
	}
	go func() { _, _ = io.Copy(io.Discard, stderr) }()
	return streamReader(ctx, stdout, session.Wait, session.Close, c.drop), nil
}

func streamReader(
	ctx context.Context,
	reader io.Reader,
	wait func() error,
	closeSession func() error,
	drop func(),
) Stream {
	lines := make(chan string)
	errors := make(chan error, 1)
	finished := make(chan struct{})
	var closeOnce sync.Once
	var closeErr error
	closeStream := func() error {
		closeOnce.Do(func() {
			closeErr = closeSession()
			drop()
		})
		return closeErr
	}

	go func() {
		select {
		case <-ctx.Done():
			_ = closeStream()
		case <-finished:
		}
	}()
	go func() {
		defer close(finished)
		defer close(lines)
		defer close(errors)
		defer closeStream()

		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			select {
			case lines <- scanner.Text():
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			errors <- err
			return
		}
		if err := wait(); err != nil && ctx.Err() == nil {
			errors <- err
		}
	}()

	return Stream{Lines: lines, Errors: errors, Close: closeStream}
}
