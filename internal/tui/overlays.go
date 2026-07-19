package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type searchOverlay struct {
	input textinput.Model
}

type overlayState struct {
	width, height int
}

func newSearchOverlay() searchOverlay {
	input := textinput.New()
	input.Placeholder = "сервер, адрес или группа"
	return searchOverlay{input: input}
}

func (m *Model) openOverlay(kind overlayKind) tea.Cmd {
	m.closeOverlay()
	m.overlay = kind
	switch kind {
	case overlayChat:
		m.chat = newChatOverlay()
		m.chat.input.Focus()
		return textinput.Blink
	case overlaySearch:
		m.search = newSearchOverlay()
		m.search.input.SetValue(m.fleet.filter.Query)
		m.search.input.Focus()
		return textinput.Blink
	case overlayPalette:
		m.palette = newPaletteOverlay()
		m.palette.refresh(*m)
		m.palette.input.Focus()
		return textinput.Blink
	case overlayHelp:
		return nil
	case overlayPassphrase:
		m.passphrase.input.Focus()
		return textinput.Blink
	default:
		return nil
	}
}

func (m *Model) closeOverlay() {
	if m.overlay == overlayChat {
		m.cancelChat()
		m.chat.messages = nil
	}
	if m.overlay == overlayPassphrase {
		m.passphrase.input.Reset()
		m.passphrase.server = ""
	}
	m.overlay = overlayNone
}

func (m *Model) resizeOverlay(width, height int) {
	m.overlayState.width, m.overlayState.height = width, height
	inputWidth := max(10, min(70, width-8))
	m.search.input.Width = inputWidth
	m.palette.input.Width = inputWidth
	m.chat.input.Width = inputWidth
	m.passphrase.input.Width = inputWidth
}

func (m Model) renderOverlay() string {
	var content string
	switch m.overlay {
	case overlayChat:
		content = m.renderChat()
	case overlaySearch:
		content = "Поиск серверов\n\n" + m.search.input.View() + "\n\nenter применить · esc закрыть"
	case overlayPalette:
		content = m.renderPalette()
	case overlayHelp:
		content = "Справка\n\n" + helpText(m.screen) + "\n\nesc закрыть"
	case overlayPassphrase:
		content = m.renderPassphrase()
	}
	box := overlayStyle.Copy().BorderStyle(lipgloss.RoundedBorder()).Padding(1, 2)
	rendered := box.Render(content)
	if m.layout.width > 0 && lipgloss.Width(rendered) > m.layout.width {
		rendered = box.Width(m.layout.width - frameOverhead).Render(content)
	}
	return rendered
}

func (m *Model) handleOverlayKey(key tea.KeyMsg) (tea.Cmd, bool) {
	if m.overlay == overlayNone {
		return nil, false
	}
	if key.String() == "esc" {
		m.closeOverlay()
		return nil, true
	}
	switch m.overlay {
	case overlayChat:
		return m.handleChatKey(key), true
	case overlaySearch:
		if key.String() == "enter" {
			m.fleet.filter.Query = m.search.input.Value()
			m.selectNearestVisible()
			m.closeOverlay()
			return nil, true
		}
		var cmd tea.Cmd
		m.search.input, cmd = m.search.input.Update(key)
		return cmd, true
	case overlayPalette:
		return m.handlePaletteKey(key), true
	case overlayPassphrase:
		return m.handlePassphraseKey(key), true
	default:
		return nil, true
	}
}

func helpText(screen screenKind) string {
	common := "c чат · : команды · ? справка · esc назад"
	switch screen {
	case screenFleet:
		return "j/k выбор · enter открыть · / поиск · g группа · ! проблемы · v превью · " + common
	case screenDashboard:
		return "r переподключить · p процессы · o порты · h история · l логи · d Docker · " + common
	case screenHistory:
		return "1-5 диапазон · j/k метрика · h/l курсор · r обновить · " + common
	case screenLogs:
		return "space пауза · / фильтр · s источник · r переподключить · " + common
	default:
		return "только чтение · " + common
	}
}
