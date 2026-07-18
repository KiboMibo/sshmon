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
	visible := filterServers(m.snapshot, m.configServers(), m.fleet.filter)
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
	visible := filterServers(m.snapshot, m.configServers(), m.fleet.filter)
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

func (m Model) configServers() []config.Server {
	if m.config == nil {
		return nil
	}
	return m.config.Servers
}

func (m Model) renderFleet() string {
	m.ensureFleet()
	visible := filterServers(m.snapshot, m.configServers(), m.fleet.filter)
	var rows strings.Builder
	rows.WriteString(titleStyle.Render("sshmon · Серверы") + "\n")
	rows.WriteString(dimStyle.Render("  СОСТ  ИМЯ             ГРУППА       CPU   MEM   LOAD   ВОЗРАСТ") + "\n")
	for _, index := range visible {
		rows.WriteString(m.renderFleetRow(index) + "\n")
	}
	if len(visible) == 0 {
		rows.WriteString(dimStyle.Render("  серверы не найдены") + "\n")
	}
	body := rows.String()
	if m.layout.wide && m.fleet.preview {
		body = lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(max(42, m.layout.width/2)).Render(strings.TrimSuffix(body, "\n")),
			"  ", m.renderFleetPreview())
	}
	return body + "\n" + dimStyle.Render("enter открыть · / поиск · g группа · ! проблемы · v вид · c чат · : команды · ? помощь · q выход")
}

func (m Model) renderFleetRow(index int) string {
	server := m.snapshot.Servers[index]
	cursor := "  "
	if index == m.selected {
		cursor = "▶ "
	}
	return fmt.Sprintf("%s%s  %-15s %-12s %4.0f%% %4.0f%% %6.2f %8s",
		cursor, statusGlyph(server), truncateCells(server.Name, 15), truncateCells(server.Group, 12),
		server.CPUPct, server.MemPct, server.Load1, metricAge(m.snapshot.Time, server.Time))
}

func (m Model) renderFleetPreview() string {
	if m.selected < 0 || m.selected >= len(m.snapshot.Servers) {
		return dimStyle.Render("сервер не выбран")
	}
	server := m.snapshot.Servers[m.selected]
	issues := issuesForServer(m.snapshot.Issues, server.Name)
	return titleStyle.Render(server.Name) + "\n" +
		fmt.Sprintf("%s  %s\nCPU  %3.0f%%  %s\nMEM  %3.0f%%  %s\nload %.2f · uptime %s\nпроблемы: %d",
			server.Hostname, statusText(server), server.CPUPct, sparkline([]float64{server.CPUPct}, 12),
			server.MemPct, sparkline([]float64{server.MemPct}, 12), server.Load1, server.Uptime.Round(time.Minute), len(issues))
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

func metricAge(now, sampled time.Time) string {
	if sampled.IsZero() {
		return "—"
	}
	if now.IsZero() {
		now = time.Now()
	}
	age := now.Sub(sampled)
	if age < 0 {
		age = 0
	}
	return fmt.Sprintf("%ds", int(age.Seconds()))
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
