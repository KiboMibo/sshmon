package setup

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"github.com/kibomibo/sshmon/internal/config"
)

// chromeHeight — строки заголовка и футера вокруг вьюпорта.
const chromeHeight = 3

func (m *model) resize(width, height int) {
	m.width, m.height = max(1, width), max(1, height)
	viewportHeight := max(1, height-chromeHeight)
	if !m.ready {
		m.viewport = viewport.New(m.width, viewportHeight)
		m.ready = true
	} else {
		m.viewport.Width = m.width
		m.viewport.Height = viewportHeight
	}
	m.refreshViewport()
}

func (m *model) ensureCursorVisible() {
	if m.cursor < m.viewport.YOffset {
		m.viewport.YOffset = m.cursor
	}
	if m.cursor >= m.viewport.YOffset+m.viewport.Height {
		m.viewport.YOffset = m.cursor - m.viewport.Height + 1
	}
	maxOffset := max(0, len(m.visible)-m.viewport.Height)
	if m.viewport.YOffset > maxOffset {
		m.viewport.YOffset = maxOffset
	}
}

func (m *model) refreshViewport() {
	if !m.ready {
		return
	}
	if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.ensureCursorVisible()
	m.viewport.SetContent(m.renderRows(m.width))
}

func (m model) View() string {
	title := styTitle.Render("sshmon: выберите серверы из SSH-конфигов")
	body := m.viewport.View()
	footer := m.footer()
	return title + "\n" + body + "\n" + footer
}

func (m model) footer() string {
	status := fmt.Sprintf("выбрано: %d", m.selectedCount())
	if m.saveBlocked {
		status = "выберите хотя бы один сервер"
	}
	return styDim.Render(status + " · s сохранить · space выбрать · enter открыть · q отмена")
}

func (m model) renderRows(width int) string {
	lines := make([]string, 0, len(m.visible))
	for vi, row := range m.visible {
		cur := "  "
		if vi == m.cursor {
			cur = "▶ "
		}
		var line string
		state := stateEmpty
		switch row.kind {
		case rowSource:
			src := m.sources[row.source]
			state = m.sourceState(row.source)
			arrow := "▸"
			if src.expanded {
				arrow = "▾"
			}
			selected := 0
			for _, h := range src.hosts {
				if h.selected {
					selected++
				}
			}
			line = fmt.Sprintf("%s%s %s %s  %d/%d",
				cur, arrow, checkGlyph(state), src.group, selected, len(src.hosts))
		case rowHost:
			h := m.sources[row.source].hosts[row.host]
			branch := "├─"
			if row.host == len(m.sources[row.source].hosts)-1 {
				branch = "└─"
			}
			mark := "□"
			if h.selected {
				mark = "■"
				state = stateChecked
			}
			line = cur + "  " + branch + " " + mark + " " + h.host.Alias + "  " + hostDesc(h.host)
		}
		line = truncateWidth(line, width)
		switch state {
		case statePartial:
			line = styPartial.Render(line)
		case stateChecked:
			line = stySelected.Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func checkGlyph(s checkState) string {
	switch s {
	case stateEmpty:
		return "□"
	case statePartial:
		return "▨"
	case stateChecked:
		return "■"
	}
	return "□"
}

func hostDesc(h config.SSHHost) string {
	desc := h.HostName
	if h.User != "" {
		desc = h.User + "@" + desc
	}
	if h.Port != 0 && h.Port != 22 {
		desc = fmt.Sprintf("%s:%d", desc, h.Port)
	}
	return desc
}

func truncateWidth(text string, width int) string {
	if lipgloss.Width(text) <= width {
		return text
	}
	limit := max(1, width-1)
	runes := []rune(text)
	for len(runes) > 0 && lipgloss.Width(string(runes)) > limit {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}
