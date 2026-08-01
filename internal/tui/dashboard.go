package tui

import (
	"fmt"
	"strings"
	"time"
)

func (m Model) renderDashboard() string {
	return m.renderDashboardWorkspace()
}

func dashboardAge(now, sampled time.Time) string {
	if sampled.IsZero() {
		return "—"
	}
	if now.IsZero() {
		now = time.Now()
	}
	age := max(time.Duration(0), now.Sub(sampled))
	if age >= time.Minute {
		return fmt.Sprintf("%dm", int(age.Minutes()))
	}
	return fmt.Sprintf("%ds", int(age.Seconds()))
}

func (m Model) dashboardIssueText(name string) string {
	issues := issuesForServer(m.snapshot.Issues, name)
	parts := make([]string, 0, len(issues))
	for _, issue := range issues {
		parts = append(parts, fmt.Sprintf("[%s] %s", issue.Severity, issue.Msg))
	}
	return strings.Join(parts, " · ")
}
