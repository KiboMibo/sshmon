package tui

import (
	"strings"
	"sync"
	"testing"

	"github.com/kibomibo/sshmon/internal/sshx"
)

type fakeConnectionManager struct {
	mu         sync.Mutex
	reconnects []string
	errors     []error
	server     string
	passphrase []byte
}

func (f *fakeConnectionManager) Reconnect(server string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reconnects = append(f.reconnects, server)
	if len(f.errors) == 0 {
		return nil
	}
	err := f.errors[0]
	f.errors = f.errors[1:]
	return err
}

func (f *fakeConnectionManager) SetPassphrase(server string, passphrase []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.server = server
	f.passphrase = append([]byte(nil), passphrase...)
	return nil
}

func TestDashboardReconnectRequestsSelectedServer(t *testing.T) {
	// Given a Dashboard with one selected server.
	connections := &fakeConnectionManager{}
	m := Model{screen: screenDashboard, snapshot: snapshotWithServers("web"), connections: connections}

	// When reconnect is requested and its asynchronous command runs.
	m, cmd := updateModel(t, m, key("r"))
	if cmd == nil {
		t.Fatal("reconnect command is nil")
	}
	msg := cmd()
	m, _ = updateModel(t, m, msg)

	// Then only the selected server is reconnected and no prompt is shown.
	if len(connections.reconnects) != 1 || connections.reconnects[0] != "web" {
		t.Fatalf("reconnects = %#v", connections.reconnects)
	}
	if m.overlay != overlayNone {
		t.Fatalf("overlay = %v", m.overlay)
	}
}

func TestPassphrasePromptMasksClearsAndRetries(t *testing.T) {
	// Given reconnect reports that the selected encrypted key needs a passphrase.
	connections := &fakeConnectionManager{errors: []error{sshx.ErrPassphraseRequired, nil}}
	m := Model{screen: screenDashboard, snapshot: snapshotWithServers("web"), connections: connections}
	m, cmd := updateModel(t, m, key("r"))
	m, _ = updateModel(t, m, cmd())
	if m.overlay != overlayPassphrase {
		t.Fatalf("overlay = %v", m.overlay)
	}

	// When a secret is typed and submitted.
	const secret = "correct horse"
	m, _ = updateModel(t, m, key(secret))
	if strings.Contains(m.View(), secret) {
		t.Fatal("rendered view contains passphrase")
	}
	m, cmd = updateModel(t, m, key("enter"))
	if cmd == nil {
		t.Fatal("retry command is nil")
	}

	// Then the input is cleared immediately, an owned copy is forwarded, and reconnect is retried.
	if m.passphrase.input.Value() != "" || m.overlay != overlayNone {
		t.Fatal("passphrase prompt retained sensitive input after submission")
	}
	if connections.server != "web" || string(connections.passphrase) != secret {
		t.Fatal("passphrase was not forwarded to the selected server")
	}
	m, _ = updateModel(t, m, cmd())
	if len(connections.reconnects) != 2 {
		t.Fatalf("reconnects = %#v", connections.reconnects)
	}
}

func TestPassphrasePromptEscapeClearsSecret(t *testing.T) {
	// Given an active passphrase prompt containing a secret.
	m := Model{overlay: overlayPassphrase, passphrase: newPassphraseOverlay("web")}
	m.passphrase.input.SetValue("never persist me")

	// When Escape closes the prompt.
	m, _ = updateModel(t, m, key("esc"))

	// Then both the overlay and secret input are cleared.
	if m.overlay != overlayNone || m.passphrase.input.Value() != "" {
		t.Fatal("passphrase prompt retained sensitive input after cancellation")
	}
}

func TestReconnectAppearsInDashboardHelpAndPalette(t *testing.T) {
	// Given Dashboard help and command palette content.
	m := Model{screen: screenDashboard, snapshot: snapshotWithServers("web")}

	// When contextual actions are built.
	items := paletteItems(m)

	// Then reconnect is discoverable in both surfaces.
	if !strings.Contains(helpText(screenDashboard), "r переподключить") {
		t.Fatal("Dashboard help misses reconnect")
	}
	found := false
	for _, item := range items {
		found = found || item.action == paletteReconnect
	}
	if !found {
		t.Fatal("Dashboard palette misses reconnect")
	}
}
