package history

import (
	"testing"

	"github.com/kibomibo/sshmon/internal/config"
)

func TestServerKeyNormalizesHostAndPort(t *testing.T) {
	// Given: an IPv6 server with mixed-case host and implicit SSH port.
	server := config.Server{User: "Deploy", Host: "2001:DB8::1"}

	// When: its stable history key is built.
	got := ServerKey(server)

	// Then: user case is preserved, host is lowercase, and port defaults to 22.
	if got != "Deploy@[2001:db8::1]:22" {
		t.Fatalf("ServerKey=%q", got)
	}
}

func TestServerKeyDefaultsEmptyUser(t *testing.T) {
	// Given: a server with implicit user and explicit port.
	server := config.Server{Host: "EXAMPLE.COM", Port: 2222}

	// When: its stable history key is built.
	got := ServerKey(server)

	// Then: the same defaults as config.Load are applied.
	if got != "root@example.com:2222" {
		t.Fatalf("ServerKey=%q", got)
	}
}
