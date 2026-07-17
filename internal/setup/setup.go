// Package setup — интерактивный выбор серверов из ~/.ssh/config при первом запуске.
package setup

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kibomibo/sshmon/internal/config"
)

var (
	styTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	styGroup  = lipgloss.NewStyle().Bold(true)
	styDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	styCursor = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	styOn     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
)

type row struct {
	header bool
	group  string         // для header
	host   config.SSHHost // для хоста
	on     bool
}

type model struct {
	rows   []row
	cursor int
	done   bool
	abort  bool
	h      int
}

// Run показывает пикер и возвращает выбранные серверы.
// Пустой срез — пользователь ничего не выбрал или отменил.
func Run(hosts []config.SSHHost) ([]config.Server, error) {
	m := newModel(hosts)
	res, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return nil, err
	}
	fm := res.(model)
	if fm.abort {
		return nil, nil
	}
	var picked []config.SSHHost
	for _, r := range fm.rows {
		if !r.header && r.on {
			picked = append(picked, r.host)
		}
	}
	return config.HostsToServers(picked), nil
}

func newModel(hosts []config.SSHHost) model {
	// группируем с сохранением порядка появления групп
	order := []string{}
	byGroup := map[string][]config.SSHHost{}
	for _, h := range hosts {
		if _, ok := byGroup[h.Group]; !ok {
			order = append(order, h.Group)
		}
		byGroup[h.Group] = append(byGroup[h.Group], h)
	}
	var rows []row
	multi := len(order) > 1 || (len(order) == 1 && order[0] != "")
	for _, g := range order {
		if multi {
			rows = append(rows, row{header: true, group: g})
		}
		for _, h := range byGroup[g] {
			rows = append(rows, row{host: h})
		}
	}
	return model{rows: rows}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.h = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.abort = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.rows)-1 {
				m.cursor++
			}
		case " ":
			m.toggle()
		case "a":
			all := true
			for _, r := range m.rows {
				if !r.header && !r.on {
					all = false
					break
				}
			}
			for i := range m.rows {
				m.rows[i].on = !all
			}
		case "enter":
			if m.countOn() > 0 {
				m.done = true
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m *model) toggle() {
	r := &m.rows[m.cursor]
	if r.header {
		// toggle всей группы: если хоть один выключен — включаем все
		anyOff := false
		for i := m.cursor + 1; i < len(m.rows) && !m.rows[i].header; i++ {
			if !m.rows[i].on {
				anyOff = true
			}
		}
		for i := m.cursor + 1; i < len(m.rows) && !m.rows[i].header; i++ {
			m.rows[i].on = anyOff
		}
		return
	}
	r.on = !r.on
}

func (m model) countOn() int {
	n := 0
	for _, r := range m.rows {
		if !r.header && r.on {
			n++
		}
	}
	return n
}

func (m model) View() string {
	var b strings.Builder
	b.WriteString(styTitle.Render("sshmon: выберите серверы для мониторинга из ~/.ssh/config") + "\n")
	b.WriteString(styDim.Render("space — выбрать (на группе — всю группу) · a — все · enter — готово · q — отмена") + "\n\n")
	for i, r := range m.rows {
		cur := "  "
		if i == m.cursor {
			cur = styCursor.Render("▶ ")
		}
		if r.header {
			name := r.group
			if name == "" {
				name = "без группы"
			}
			b.WriteString(cur + styGroup.Render("["+name+"]") + "\n")
			continue
		}
		mark := styDim.Render("[ ]")
		if r.on {
			mark = styOn.Render("[x]")
		}
		desc := r.host.HostName
		if r.host.User != "" {
			desc = r.host.User + "@" + desc
		}
		if r.host.Port != 0 && r.host.Port != 22 {
			desc += fmt.Sprintf(":%d", r.host.Port)
		}
		b.WriteString(cur + mark + " " + r.host.Alias + "  " + styDim.Render(desc) + "\n")
	}
	b.WriteString("\n" + styDim.Render(fmt.Sprintf("выбрано: %d", m.countOn())) + "\n")
	return b.String()
}
