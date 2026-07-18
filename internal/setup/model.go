package setup

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/kibomibo/sshmon/internal/config"
)

type rowKind int

const (
	rowSource rowKind = iota
	rowHost
)

type checkState int

const (
	stateEmpty checkState = iota
	statePartial
	stateChecked
)

type hostNode struct {
	host     config.SSHHost
	selected bool
}

type sourceNode struct {
	path     string
	group    string
	hosts    []hostNode
	expanded bool
}

type visibleRow struct {
	kind   rowKind
	source int
	host   int
}

type model struct {
	sources     []sourceNode
	visible     []visibleRow
	cursor      int
	done        bool
	abort       bool
	saveBlocked bool
	viewport    viewport.Model
	ready       bool
	width       int
	height      int
}

// newModel группирует хосты по источникам в порядке первого появления SourcePath.
func newModel(hosts []config.SSHHost) model {
	m := model{}
	idx := map[string]int{}
	for _, h := range hosts {
		i, ok := idx[h.SourcePath]
		if !ok {
			i = len(m.sources)
			idx[h.SourcePath] = i
			m.sources = append(m.sources, sourceNode{path: h.SourcePath, group: h.Group})
		}
		m.sources[i].hosts = append(m.sources[i].hosts, hostNode{host: h})
	}
	m.rebuildVisible()
	return m
}

func (m *model) rebuildVisible() {
	m.visible = m.visible[:0]
	for si := range m.sources {
		m.visible = append(m.visible, visibleRow{kind: rowSource, source: si})
		if m.sources[si].expanded {
			for hi := range m.sources[si].hosts {
				m.visible = append(m.visible, visibleRow{kind: rowHost, source: si, host: hi})
			}
		}
	}
	if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m model) sourceState(i int) checkState {
	selected := 0
	for _, h := range m.sources[i].hosts {
		if h.selected {
			selected++
		}
	}
	switch {
	case selected == 0:
		return stateEmpty
	case selected == len(m.sources[i].hosts):
		return stateChecked
	default:
		return statePartial
	}
}

func (m *model) toggleSource(i int) {
	turnOn := m.sourceState(i) != stateChecked
	for hi := range m.sources[i].hosts {
		m.sources[i].hosts[hi].selected = turnOn
	}
}

func (m *model) toggleCurrent() {
	if len(m.visible) == 0 {
		return
	}
	row := m.visible[m.cursor]
	switch row.kind {
	case rowSource:
		m.toggleSource(row.source)
	case rowHost:
		h := &m.sources[row.source].hosts[row.host]
		h.selected = !h.selected
	}
}

// toggleExpanded сворачивает/разворачивает источник и ставит курсор на его строку.
func (m *model) toggleExpanded(i int) {
	m.sources[i].expanded = !m.sources[i].expanded
	m.rebuildVisible()
	for vi, row := range m.visible {
		if row.kind == rowSource && row.source == i {
			m.cursor = vi
			return
		}
	}
}

func (m *model) toggleAll() {
	turnOn := false
	for i := range m.sources {
		if m.sourceState(i) != stateChecked {
			turnOn = true
			break
		}
	}
	for si := range m.sources {
		for hi := range m.sources[si].hosts {
			m.sources[si].hosts[hi].selected = turnOn
		}
	}
}

// selectedHosts возвращает выбранные хосты в порядке источников и объявлений.
func (m model) selectedHosts() []config.SSHHost {
	var out []config.SSHHost
	for _, s := range m.sources {
		for _, h := range s.hosts {
			if h.selected {
				out = append(out, h.host)
			}
		}
	}
	return out
}

func (m model) selectedCount() int {
	n := 0
	for _, s := range m.sources {
		for _, h := range s.hosts {
			if h.selected {
				n++
			}
		}
	}
	return n
}

func (m *model) move(delta int) {
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}
}

func (m *model) moveToParent() {
	row := m.visible[m.cursor]
	for vi, r := range m.visible {
		if r.kind == rowSource && r.source == row.source {
			m.cursor = vi
			return
		}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.resize(size.Width, size.Height)
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	m.saveBlocked = false
	switch key.String() {
	case "q", "esc", "ctrl+c":
		m.abort = true
		return m, tea.Quit
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "enter":
		if len(m.visible) == 0 {
			break
		}
		row := m.visible[m.cursor]
		switch row.kind {
		case rowSource:
			m.toggleExpanded(row.source)
		case rowHost:
			m.toggleCurrent()
		}
	case "right", "l":
		if len(m.visible) == 0 {
			break
		}
		row := m.visible[m.cursor]
		if row.kind == rowSource && !m.sources[row.source].expanded {
			m.toggleExpanded(row.source)
		}
	case "left", "h":
		if len(m.visible) == 0 {
			break
		}
		row := m.visible[m.cursor]
		switch row.kind {
		case rowHost:
			m.moveToParent()
		case rowSource:
			if m.sources[row.source].expanded {
				m.toggleExpanded(row.source)
			}
		}
	case " ":
		m.toggleCurrent()
	case "a":
		m.toggleAll()
	case "s":
		if m.selectedCount() == 0 {
			m.saveBlocked = true
			break
		}
		m.done = true
		return m, tea.Quit
	}
	m.refreshViewport()
	return m, nil
}
