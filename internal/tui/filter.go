package tui

import (
	"strings"

	"github.com/kibomibo/sshmon/internal/collect"
	"github.com/kibomibo/sshmon/internal/config"
)

type fleetFilter struct {
	Query        string
	Group        string
	ProblemsOnly bool
}

func filterServers(snapshot collect.Snapshot, servers []config.Server, filter fleetFilter) []int {
	issues := make(map[string]bool, len(snapshot.Issues))
	for _, issue := range snapshot.Issues {
		issues[issue.Server] = true
	}
	query := strings.ToLower(strings.TrimSpace(filter.Query))
	indices := make([]int, 0, len(snapshot.Servers))
	for i, metrics := range snapshot.Servers {
		if filter.Group != "" && metrics.Group != filter.Group {
			continue
		}
		if filter.ProblemsOnly && !issues[metrics.Name] {
			continue
		}
		host := ""
		if i < len(servers) {
			host = servers[i].Host
		}
		haystack := strings.ToLower(metrics.Name + "\n" + host + "\n" + metrics.Group)
		if query != "" && !strings.Contains(haystack, query) {
			continue
		}
		indices = append(indices, i)
	}
	return indices
}

func cycleGroup(current string, servers []collect.Metrics) string {
	groups := make([]string, 0)
	seen := make(map[string]bool)
	for _, server := range servers {
		if server.Group == "" || seen[server.Group] {
			continue
		}
		seen[server.Group] = true
		groups = append(groups, server.Group)
	}
	if current == "" {
		if len(groups) == 0 {
			return ""
		}
		return groups[0]
	}
	for i, group := range groups {
		if group == current && i+1 < len(groups) {
			return groups[i+1]
		}
	}
	return ""
}
