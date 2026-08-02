package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kibomibo/sshmon/internal/collect"
)

type fleetCounts struct {
	ok   int
	warn int
	down int
}

func (c fleetCounts) total() int { return c.ok + c.warn + c.down }

func countStates(servers []collect.Metrics, issues []collect.Issue) fleetCounts {
	problem := make(map[string]bool, len(issues))
	for _, issue := range issues {
		problem[issue.Server] = true
	}
	var counts fleetCounts
	for _, server := range servers {
		switch {
		case !server.Online:
			counts.down++
		case problem[server.Name]:
			counts.warn++
		default:
			counts.ok++
		}
	}
	return counts
}

func fleetGroupSummary(servers []collect.Metrics, issues []collect.Issue) ([]string, map[string]fleetCounts) {
	order := make([]string, 0)
	byGroup := make(map[string][]collect.Metrics)
	for _, server := range servers {
		if server.Group == "" {
			continue
		}
		if _, seen := byGroup[server.Group]; !seen {
			order = append(order, server.Group)
		}
		byGroup[server.Group] = append(byGroup[server.Group], server)
	}
	counts := make(map[string]fleetCounts, len(order))
	for group, list := range byGroup {
		counts[group] = countStates(list, issues)
	}
	return order, counts
}

func fleetTiles(servers []collect.Metrics, issues []collect.Issue, active string) []string {
	order, counts := fleetGroupSummary(servers, issues)
	tiles := make([]string, 0, len(order)+1)
	for _, group := range order {
		tiles = append(tiles, fleetTile(group, counts[group], active == group))
	}
	return append(tiles, fleetTile("всё", countStates(servers, issues), active == ""))
}

func fleetTile(label string, counts fleetCounts, active bool) string {
	style, shape := dimStyle, lipgloss.RoundedBorder()
	if active {
		green := goodStyle.BorderForeground(lipgloss.Color("42"))
		style, shape = green, lipgloss.DoubleBorder()
	}
	title := fmt.Sprintf("%s %d", label, counts.total())
	glyphs := tileGlyphs(counts)
	if glyphs == "" {
		glyphs = style.Render("—")
	}
	body := lipgloss.JoinVertical(lipgloss.Center,
		style.Bold(true).Render(title),
		glyphs,
	)
	return style.BorderStyle(shape).Padding(1, 2).Align(lipgloss.Center).Render(body)
}

func tileGlyphs(counts fleetCounts) string {
	parts := make([]string, 0, 3)
	if counts.ok > 0 {
		parts = append(parts, goodStyle.Render("●")+strconv.Itoa(counts.ok))
	}
	if counts.warn > 0 {
		parts = append(parts, warnStyle.Render("⚠")+strconv.Itoa(counts.warn))
	}
	if counts.down > 0 {
		parts = append(parts, criticalStyle.Render("×")+strconv.Itoa(counts.down))
	}
	return strings.Join(parts, " ")
}

func packTiles(tiles []string, width int) []string {
	lines := make([]string, 0, 3)
	row := make([]string, 0, len(tiles))
	rowWidth := 0
	flush := func() {
		if len(row) == 0 {
			return
		}
		lines = append(lines, strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, row...), "\n")...)
		row, rowWidth = row[:0], 0
	}
	for _, tile := range tiles {
		tileWidth := lipgloss.Width(tile)
		if rowWidth > 0 && rowWidth+1+tileWidth > width {
			flush()
		}
		if rowWidth > 0 {
			row = append(row, " ")
			rowWidth++
		}
		row = append(row, tile)
		rowWidth += tileWidth
	}
	flush()
	return lines
}

func (m Model) fleetGroupBox(width int) []string {
	tiles := fleetTiles(m.snapshot.Servers, m.snapshot.Issues, m.fleet.filter.Group)
	if len(tiles) < 2 {
		return nil
	}
	// Общая рамка «ГРУППЫ» по макету: без неё ряд плиток читается как продолжение
	// шапки, а не как отдельный переключатель области видимости.
	return panelBoxStyled("ГРУППЫ", "", width, packTiles(tiles, width-4), dimStyle)
}

func (m Model) fleetHeader(width int) string {
	counts := countStates(m.snapshot.Servers, m.snapshot.Issues)
	left := fmt.Sprintf("%s  %s   %s", titleStyle.Render("FLEET"), hostsText(counts.total()), stateSummary(counts))
	return spread(left, dimStyle.Render(m.pollHint()), width)
}

func stateSummary(counts fleetCounts) string {
	return fmt.Sprintf("%s %d норма   %s %d внимание   %s %d нет связи",
		goodStyle.Render("●"), counts.ok,
		warnStyle.Render("⚠"), counts.warn,
		criticalStyle.Render("×"), counts.down)
}

func (m Model) pollHint() string {
	if m.config == nil || m.config.Interval <= 0 {
		return ""
	}
	hint := "опрос " + formatShortDuration(m.config.Interval)
	if !m.snapshot.Time.IsZero() {
		hint += " · " + formatShortDuration(time.Since(m.snapshot.Time))
	}
	return hint
}

func formatShortDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%.0fс", max(0, d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%.0fм", d.Minutes())
	default:
		return fmt.Sprintf("%.0fч", d.Hours())
	}
}

func (m Model) fleetGroupTotal() int {
	return len(filterServers(m.snapshot, m.configServers(), fleetFilter{Group: m.fleet.filter.Group}))
}

func (m Model) fleetContextLine(visible, groupTotal, width int) string {
	scope := m.fleet.filter.Group
	if scope == "" {
		scope = "всё"
	}
	left := fmt.Sprintf("%s · %s", titleStyle.Render(scope), hostsText(groupTotal))
	if query := m.fleet.filter.Query; query != "" || m.fleet.searching {
		left += "   поиск > " + query
		if m.fleet.searching {
			left += "|"
		}
	}
	if visible != groupTotal {
		left += fmt.Sprintf("   %d из %d", visible, groupTotal)
	}
	return spread(left, dimStyle.Render("tab группа · a всё · esc сброс"), width)
}

func (m Model) fleetHiddenNote(visible, groupTotal int) string {
	hidden := groupTotal - visible
	if hidden <= 0 {
		return ""
	}
	if query := m.fleet.filter.Query; query != "" {
		return dimStyle.Render(fmt.Sprintf("%d скрыто фильтром «%s»", hidden, query))
	}
	return dimStyle.Render(fmt.Sprintf("%d скрыто фильтром", hidden))
}

func (m *Model) openFleetSearch() tea.Cmd {
	m.ensureFleet()
	m.fleet.searching = true
	m.search = newSearchOverlay()
	m.search.input.SetValue(m.fleet.filter.Query)
	m.search.input.Focus()
	return textinput.Blink
}

func (m *Model) handleFleetSearchKey(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case "esc":
		m.fleet.searching = false
		m.search.input.Reset()
		m.fleet.filter.Query = ""
		m.selectNearestVisible()
		return nil
	case "enter":
		m.fleet.searching = false
		m.search.input.Blur()
		return nil
	}
	var cmd tea.Cmd
	m.search.input, cmd = m.search.input.Update(key)
	m.fleet.filter.Query = m.search.input.Value()
	m.selectNearestVisible()
	return cmd
}

func spread(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return fitLine(left, width)
	}
	return left + strings.Repeat(" ", gap) + right
}
