package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/kibomibo/sshmon/internal/collect"
	"github.com/kibomibo/sshmon/internal/config"
)

const fleetPageSize = 10

type fleetModel struct {
	filter      fleetFilter
	preview     bool
	initialized bool
}

func newFleetModel() fleetModel {
	return fleetModel{preview: true, initialized: true}
}

func (m *Model) ensureFleet() {
	if !m.fleet.initialized {
		m.fleet = newFleetModel()
	}
}

func (m *Model) moveFleet(delta int) {
	visible := groupedServers(m.snapshot, m.configServers(), m.fleet.filter)
	if len(visible) == 0 {
		return
	}
	position := nearestPosition(visible, m.selected)
	position += delta
	if position < 0 {
		position = 0
	}
	if position >= len(visible) {
		position = len(visible) - 1
	}
	m.selected = visible[position]
}

func (m *Model) selectNearestVisible() {
	visible := groupedServers(m.snapshot, m.configServers(), m.fleet.filter)
	if len(visible) == 0 {
		return
	}
	m.selected = visible[nearestPosition(visible, m.selected)]
}

func nearestPosition(indices []int, selected int) int {
	best := 0
	bestDistance := abs(indices[0] - selected)
	for i := 1; i < len(indices); i++ {
		distance := abs(indices[i] - selected)
		if distance < bestDistance {
			best, bestDistance = i, distance
		}
	}
	return best
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func fleetRowStyle(selected bool) lipgloss.Style {
	if selected {
		return focusStyle
	}
	return dimStyle
}

func fleetScroll(selectedRow, height, total int) int {
	if total <= height {
		return 0
	}
	scroll := selectedRow - height/2
	if scroll < 0 {
		scroll = 0
	}
	if scroll > total-height {
		scroll = total - height
	}
	return scroll
}

func groupedServers(snapshot collect.Snapshot, servers []config.Server, filter fleetFilter) []int {
	visible := filterServers(snapshot, servers, filter)
	order := make([]string, 0)
	buckets := make(map[string][]int)
	for _, index := range visible {
		group := snapshot.Servers[index].Group
		if _, seen := buckets[group]; !seen {
			order = append(order, group)
		}
		buckets[group] = append(buckets[group], index)
	}
	grouped := make([]int, 0, len(visible))
	for _, group := range order {
		grouped = append(grouped, buckets[group]...)
	}
	return grouped
}

func (m Model) configServers() []config.Server {
	if m.config == nil {
		return nil
	}
	return m.config.Servers
}

func (m Model) renderFleet() string {
	m.ensureFleet()
	footer := dimStyle.Render("enter открыть · / поиск · g группа · ! проблемы · v вид · c чат · : команды · ? помощь · q выход")
	if m.layout.wide {
		return m.renderFleetWide() + "\n" + footer
	}
	listLines, _ := m.fleetListLines()
	body := titleStyle.Render("sshmon · Серверы") + "\n" + strings.Join(listLines, "\n")
	return body + "\n" + footer
}

func (m Model) renderFleetWide() string {
	listLines, selectedRow := m.fleetListLines()
	contentH := max(1, m.layout.height-3)
	scroll := fleetScroll(selectedRow, contentH, len(listLines))
	if !m.fleet.preview {
		full := panelBoxStyled("СЕРВЕРЫ", "v детали · / поиск · g группа · ! проблемы", m.layout.width,
			fitPanelHeight(listLines, contentH, scroll), dimStyle)
		return strings.Join(full, "\n")
	}
	rightW := max(30, m.layout.width/4)
	leftW := m.layout.width - rightW - 2
	left := panelBoxStyled("СЕРВЕРЫ", "enter открыть · v свернуть · / поиск", leftW,
		fitPanelHeight(listLines, contentH, scroll), dimStyle)
	right := panelBoxStyled(m.fleetDetailTitle(), "! проблемы · g группа", rightW,
		fitPanelHeight(m.fleetDetailContent(rightW-4), contentH, 0), dimStyle)
	return joinBoxes(left, right)
}

func (m Model) fleetListLines() ([]string, int) {
	visible := groupedServers(m.snapshot, m.configServers(), m.fleet.filter)
	lines := []string{dimStyle.Render("  СОСТ  ИМЯ             CPU   MEM   LOAD   UPTIME")}
	selectedRow := 0
	previousGroup := ""
	for _, index := range visible {
		if group := m.snapshot.Servers[index].Group; group != "" && group != previousGroup {
			lines = append(lines, titleStyle.Render(group))
			previousGroup = group
		}
		if index == m.selected {
			selectedRow = len(lines)
		}
		lines = append(lines, m.renderFleetRow(index))
	}
	if len(visible) == 0 {
		lines = append(lines, dimStyle.Render("  серверы не найдены"))
	}
	return lines, selectedRow
}

func (m Model) renderFleetRow(index int) string {
	server := m.snapshot.Servers[index]
	cursor := "  "
	if index == m.selected {
		cursor = "▶ "
	}
	row := fmt.Sprintf("%s%s  %-15s %4.0f%% %4.0f%% %6.2f %8s",
		cursor, statusGlyph(server), truncateCells(server.Name, 15),
		server.CPUPct, server.MemPct, server.Load1, formatUptime(server.Uptime))
	return fleetRowStyle(index == m.selected).Render(row)
}

func statusGlyph(server collect.Metrics) string {
	if server.Time.IsZero() {
		return dimStyle.Render("◌")
	}
	if !server.Online {
		return criticalStyle.Render("×")
	}
	return goodStyle.Render("●")
}

func statusText(server collect.Metrics) string {
	if server.Time.IsZero() {
		return "ожидание"
	}
	if !server.Online {
		return "недоступен"
	}
	return "доступен"
}

func formatUptime(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	hours := int(d.Hours())
	if hours >= 24 {
		return fmt.Sprintf("%dd%dh", hours/24, hours%24)
	}
	return fmt.Sprintf("%dh%dm", hours, int(d.Minutes())%60)
}

func issuesForServer(issues []collect.Issue, name string) []collect.Issue {
	result := make([]collect.Issue, 0)
	for _, issue := range issues {
		if issue.Server == name {
			result = append(result, issue)
		}
	}
	return result
}

func truncateCells(value string, width int) string {
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	return string(runes[:max(1, width-1)]) + "…"
}
