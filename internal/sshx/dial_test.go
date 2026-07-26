package sshx

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/kibomibo/sshmon/internal/config"
)

func TestConnAbortsOnCancel(t *testing.T) {
	// Given a listener that accepts TCP but never answers the SSH handshake.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer ln.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		accepted <- conn
	}()
	host, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort() error = %v", err)
	}
	portNum, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("Atoi() error = %v", err)
	}
	client := New(config.Server{
		Name: "hang", Host: host, Port: portNum, User: "u",
		Password: "x", InsecureHostKey: true,
	})
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := client.conn(ctx)
		result <- err
	}()
	conn := <-accepted
	defer conn.Close()

	// When the context is cancelled while the handshake is stuck.
	cancel()

	// Then dialing gives up instead of blocking on the read.
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("conn() succeeded against a silent server")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("conn() did not abort on cancellation")
	}
}
