package tui

import (
	"reflect"
	"testing"

	"github.com/kibomibo/sshmon/internal/collect"
	"github.com/kibomibo/sshmon/internal/config"
)

func TestFilterServersPreservesOrderAndMatchesAllFields(t *testing.T) {
	// Given servers whose name, host and group match different queries.
	snapshot := collect.Snapshot{Servers: []collect.Metrics{{Name: "web", Group: "prod"}, {Name: "db", Group: "data"}, {Name: "cache", Group: "prod"}}}
	servers := []config.Server{{Name: "web", Host: "10.0.0.1", Group: "prod"}, {Name: "db", Host: "postgres.internal", Group: "data"}, {Name: "cache", Host: "10.0.0.3", Group: "prod"}}
	// When filters target host and group.
	byHost := filterServers(snapshot, servers, fleetFilter{Query: "POSTGRES"})
	byGroup := filterServers(snapshot, servers, fleetFilter{Group: "prod"})
	// Then original config order is retained.
	if !reflect.DeepEqual(byHost, []int{1}) || !reflect.DeepEqual(byGroup, []int{0, 2}) {
		t.Fatalf("host=%v group=%v", byHost, byGroup)
	}
}

func TestFilterServersProblemsOnlyUsesSnapshotIssues(t *testing.T) {
	// Given one server with a detected issue.
	snapshot := collect.Snapshot{Servers: []collect.Metrics{{Name: "web"}, {Name: "db"}}, Issues: []collect.Issue{{Server: "db", Severity: "crit", Msg: "offline"}}}
	// When the problems-only filter is enabled.
	indices := filterServers(snapshot, nil, fleetFilter{ProblemsOnly: true})
	// Then only the affected server survives.
	if !reflect.DeepEqual(indices, []int{1}) {
		t.Fatalf("indices = %v", indices)
	}
}

func TestCycleGroupIsDeterministic(t *testing.T) {
	// Given repeated groups in config order.
	servers := []collect.Metrics{{Group: "prod"}, {Group: "data"}, {Group: "prod"}, {Group: ""}}
	// When group filtering is cycled.
	first := cycleGroup("", servers)
	second := cycleGroup(first, servers)
	third := cycleGroup(second, servers)
	// Then unique groups cycle in first-seen order and return to all.
	if first != "prod" || second != "data" || third != "" {
		t.Fatalf("cycle = %q, %q, %q", first, second, third)
	}
}
