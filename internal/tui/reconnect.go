package tui

import (
	"errors"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/kibomibo/sshmon/internal/collect"
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
	if errors.Is(msg.err, collect.ErrPassphraseRequired) || errors.Is(msg.err, collect.ErrInvalidPassphrase) {
		// Реконнект запускается и из палитры, и из чата: предыдущий оверлей
		// нельзя оставлять недоделанным. У чата достаточно отменить активный
		// запрос — переписка не имеет отношения к парольной фразе и переживает
		// запрос; остальные оверлеи закрываем полностью.
		if m.overlay == overlayChat {
			m.cancelChat()
			m.overlay = overlayNone
		} else {
			m.closeOverlay()
		}
		m.passphrase = newPassphraseOverlay(msg.server)
		m.overlay = overlayPassphrase
	}
}

func (m *Model) handlePassphraseKey(key tea.KeyMsg) tea.Cmd {
	if key.String() == "enter" {
		if m.connections == nil {
			return nil
		}
		// Одна строка от textinput и один байтовый буфер — больше копий секрета
		// в куче не появляется, буфер зануляем сразу после передачи (получатель
		// хранит собственную копию). Остаточный риск: внутренний []rune самого
		// textinput не затирается — Reset лишь отбрасывает срез, и секрет живёт
		// в куче до сборки мусора.
		secret := []byte(m.passphrase.input.Value())
		m.passphrase.input.Reset()
		if len(secret) == 0 {
			return nil
		}
		err := m.connections.SetPassphrase(m.passphrase.server, secret)
		for index := range secret {
			secret[index] = 0
		}
		if err != nil {
			return nil
		}
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
