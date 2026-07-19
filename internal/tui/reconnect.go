package tui

import (
	"errors"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/kibomibo/sshmon/internal/sshx"
)

type connectionManager interface {
	Reconnect(string) error
	SetPassphrase(string, []byte) error
}

type passphraseOverlay struct {
	input  textinput.Model
	server string
}

type reconnectResultMsg struct {
	server     string
	generation uint64
	err        error
}

func newPassphraseOverlay(server string) passphraseOverlay {
	input := textinput.New()
	input.Placeholder = "passphrase"
	input.EchoMode = textinput.EchoPassword
	input.EchoCharacter = '•'
	input.Focus()
	return passphraseOverlay{input: input, server: server}
}

func (m *Model) startReconnect() tea.Cmd {
	if m.connections == nil || m.selected < 0 || m.selected >= len(m.snapshot.Servers) {
		return nil
	}
	m.reconnectGeneration++
	server := m.snapshot.Servers[m.selected].Name
	generation := m.reconnectGeneration
	connections := m.connections
	return func() tea.Msg {
		return reconnectResultMsg{server: server, generation: generation, err: connections.Reconnect(server)}
	}
}

func (m *Model) applyReconnectResult(msg reconnectResultMsg) {
	if msg.generation != m.reconnectGeneration {
		return
	}
	if errors.Is(msg.err, sshx.ErrPassphraseRequired) || errors.Is(msg.err, sshx.ErrInvalidPassphrase) {
		m.passphrase = newPassphraseOverlay(msg.server)
		m.overlay = overlayPassphrase
	}
}

func (m *Model) handlePassphraseKey(key tea.KeyMsg) tea.Cmd {
	if key.String() == "enter" {
		value := m.passphrase.input.Value()
		if value == "" || m.connections == nil {
			return nil
		}
		secret := []byte(value)
		err := m.connections.SetPassphrase(m.passphrase.server, secret)
		for index := range secret {
			secret[index] = 0
		}
		if err != nil {
			m.passphrase.input.Reset()
			return nil
		}
		m.passphrase.input.Reset()
		m.overlay = overlayNone
		return m.startReconnect()
	}
	var cmd tea.Cmd
	m.passphrase.input, cmd = m.passphrase.input.Update(key)
	return cmd
}

func (m Model) renderPassphrase() string {
	return "Passphrase для ключа · " + m.passphrase.server + "\n\n" +
		m.passphrase.input.View() + "\n\nenter подключиться · esc отмена"
}
