package collect

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kibomibo/sshmon/internal/config"
	"github.com/kibomibo/sshmon/internal/sshx"
)

type fakePollRunner struct {
	mu         sync.Mutex
	output     string
	err        error
	resetCalls int
	passphrase []byte
}

func (f *fakePollRunner) Run(string, time.Duration) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.output, f.err
}

func (f *fakePollRunner) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resetCalls++
}

func (f *fakePollRunner) SetPassphrase(passphrase []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.passphrase = append([]byte(nil), passphrase...)
}

func TestReconnectRejectsUnknownServer(t *testing.T) {
	// Given a collector without the requested server.
	collector := &Collector{cfg: &config.Config{}}

	// When reconnect is requested for an unknown name.
	err := collector.Reconnect("missing")

	// Then the request fails without touching a connection.
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("Reconnect error = %v, want unknown server name", err)
	}
}

func TestReconnectResetsPollsAndPublishesSelectedServer(t *testing.T) {
	// Given a selected server backed by a deterministic SSH runner.
	runner := &fakePollRunner{output: rawFixture}
	collector := newReconnectTestCollector("web", runner)
	events, unsubscribe := collector.Subscribe(1)
	defer unsubscribe()

	// When the server is reconnected.
	err := collector.Reconnect("web")

	// Then its cached connection is reset, sampled immediately, and published.
	if err != nil {
		t.Fatalf("Reconnect() error = %v", err)
	}
	if runner.resetCalls != 1 {
		t.Fatalf("Reset calls = %d, want 1", runner.resetCalls)
	}
	select {
	case event := <-events:
		if len(event.Snapshot.Servers) != 1 || !event.Snapshot.Servers[0].Online {
			t.Fatalf("published snapshot = %#v", event.Snapshot.Servers)
		}
	case <-time.After(time.Second):
		t.Fatal("reconnect snapshot was not published")
	}
}

func TestReconnectPropagatesPassphraseRequirement(t *testing.T) {
	// Given an encrypted key that cannot connect without a passphrase.
	runner := &fakePollRunner{err: sshx.ErrPassphraseRequired}
	collector := newReconnectTestCollector("web", runner)

	// When the server is reconnected.
	err := collector.Reconnect("web")

	// Then the typed error reaches the TUI and the snapshot contains no secret.
	if !errors.Is(err, sshx.ErrPassphraseRequired) {
		t.Fatalf("Reconnect() error = %v, want ErrPassphraseRequired", err)
	}
	metrics := collector.Snapshot().Servers[0]
	if metrics.Online || metrics.Err == "" {
		t.Fatalf("metrics = %#v, want generic offline error", metrics)
	}
}

func TestSetPassphraseForwardsOwnedSecretToSelectedServer(t *testing.T) {
	// Given a collector with one selected server.
	runner := &fakePollRunner{}
	collector := newReconnectTestCollector("web", runner)
	secret := []byte("correct horse")

	// When a passphrase is supplied and the caller clears its buffer.
	err := collector.SetPassphrase("web", secret)
	clear(secret)

	// Then the selected client owns an independent in-memory copy.
	if err != nil {
		t.Fatalf("SetPassphrase() error = %v", err)
	}
	if got := string(runner.passphrase); got != "correct horse" {
		t.Fatal("runner did not retain an independent passphrase copy")
	}
}

func newReconnectTestCollector(name string, runner pollRunner) *Collector {
	server := config.Server{Name: name}
	return &Collector{
		cfg:         &config.Config{},
		states:      []*serverState{{cfg: server, runner: runner, m: Metrics{Name: name}}},
		subscribers: make(map[uint64]chan Event),
	}
}
