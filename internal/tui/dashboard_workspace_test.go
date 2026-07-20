package tui

import (
	"context"
	"errors"
	"slices"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kibomibo/sshmon/internal/collect"
	"github.com/kibomibo/sshmon/internal/config"
)

type fakeDashboardSource struct {
	containers   []collect.Container
	units        []collect.SystemdUnit
	lines        []string
	containerErr error
	unitErr      error
	logErr       error
	unitNames    []string
	logRequests  []collect.LogRequest
	logLines     []int
}

func (f *fakeDashboardSource) Containers(ctx context.Context, _ string) ([]collect.Container, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return slices.Clone(f.containers), f.containerErr
}

func (f *fakeDashboardSource) SystemdUnits(ctx context.Context, _ string, names []string) ([]collect.SystemdUnit, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.unitNames = slices.Clone(names)
	return slices.Clone(f.units), f.unitErr
}

func (f *fakeDashboardSource) LogSnapshot(ctx context.Context, request collect.LogRequest, lines int) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.logRequests = append(f.logRequests, request)
	f.logLines = append(f.logLines, lines)
	return slices.Clone(f.lines), f.logErr
}

func TestDashboardWorkspaceStartsIndependentOneShotRequests(t *testing.T) {
	// Given a selected server and exact systemd units from configuration.
	source := &fakeDashboardSource{
		containers: []collect.Container{{Name: "api"}},
		units:      []collect.SystemdUnit{{Name: "nginx.service"}},
		lines:      []string{"booted"},
	}
	m := Model{
		screen:          screenDashboard,
		snapshot:        snapshotWithServers("web"),
		config:          &config.Config{Dashboard: config.Dashboard{SystemdUnits: []string{"nginx.service"}}},
		dashboardSource: source,
	}

	// When the workspace starts and all three one-shot commands complete.
	cmd := m.startDashboardWorkspace()
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 3 {
		t.Fatalf("batch = %T len=%d, want tea.BatchMsg with 3 commands", cmd(), len(batch))
	}
	if m.dashboard.containers.status != diagnosticsLoading || m.dashboard.units.status != diagnosticsLoading || m.dashboard.logs.status != diagnosticsLoading {
		t.Fatalf("statuses = %v/%v/%v", m.dashboard.containers.status, m.dashboard.units.status, m.dashboard.logs.status)
	}
	if m.dashboard.containers.generation == 0 || m.dashboard.units.generation == 0 || m.dashboard.logs.generation == 0 {
		t.Fatalf("generations = %d/%d/%d", m.dashboard.containers.generation, m.dashboard.units.generation, m.dashboard.logs.generation)
	}
	for _, request := range batch {
		m, _ = updateModel(t, m, request())
	}

	// Then each source is ready, configured units are exact, and logs use a static 50-line system request.
	if m.dashboard.containers.status != diagnosticsReady || m.dashboard.units.status != diagnosticsReady || m.dashboard.logs.status != diagnosticsReady {
		t.Fatalf("statuses after results = %v/%v/%v", m.dashboard.containers.status, m.dashboard.units.status, m.dashboard.logs.status)
	}
	if !slices.Equal(source.unitNames, []string{"nginx.service"}) {
		t.Fatalf("unit names = %#v", source.unitNames)
	}
	if len(source.logRequests) != 1 || source.logRequests[0].Source.Kind != collect.LogSystem || source.logLines[0] != 50 {
		t.Fatalf("log calls = %#v lines=%#v", source.logRequests, source.logLines)
	}
}

func TestDashboardWorkspaceIgnoresStaleResultPerSource(t *testing.T) {
	// Given independent current generations and existing sibling data.
	m := Model{dashboard: dashboardWorkspace{
		containers: dashboardContainersState{generation: 10, items: []collect.Container{{Name: "current"}}},
		units:      dashboardUnitsState{generation: 20, items: []collect.SystemdUnit{{Name: "sshd.service"}}},
		logs:       dashboardLogState{generation: 30, lines: []string{"current log"}},
	}}

	// When stale container and current failing unit results arrive.
	m, staleCmd := updateModel(t, m, dashboardContainersResultMsg{generation: 9, items: []collect.Container{{Name: "stale"}}})
	unitErr := errors.New("systemctl failed")
	m, unitCmd := updateModel(t, m, dashboardUnitsResultMsg{generation: 20, err: unitErr})

	// Then the stale source is unchanged, the current failure is isolated, and no refresh is scheduled.
	if staleCmd != nil || unitCmd != nil {
		t.Fatal("dashboard workspace scheduled an automatic refresh")
	}
	if got := m.dashboard.containers.items[0].Name; got != "current" {
		t.Fatalf("container = %q", got)
	}
	if !errors.Is(m.dashboard.units.err, unitErr) || m.dashboard.logs.lines[0] != "current log" {
		t.Fatalf("unit err=%v log=%#v", m.dashboard.units.err, m.dashboard.logs.lines)
	}
}

func TestDashboardWorkspaceRestartCancelsPreviousRequests(t *testing.T) {
	// Given an initial workspace load whose commands have not run yet.
	m := Model{screen: screenDashboard, snapshot: snapshotWithServers("web"), dashboardSource: &fakeDashboardSource{}}
	first := m.startDashboardWorkspace()
	firstBatch := first().(tea.BatchMsg)

	// When the workspace restarts before the old commands execute.
	m.startDashboardWorkspace()
	oldResult := firstBatch[0]().(dashboardContainersResultMsg)

	// Then the previous request observes cancellation and cannot replace current state.
	if !errors.Is(oldResult.err, context.Canceled) {
		t.Fatalf("old result err = %v, want context canceled", oldResult.err)
	}
	currentGeneration := m.dashboard.containers.generation
	m, _ = updateModel(t, m, oldResult)
	if m.dashboard.containers.generation != currentGeneration || m.dashboard.containers.status != diagnosticsLoading {
		t.Fatalf("current container state changed: %#v", m.dashboard.containers)
	}
}

func TestDashboardLogSwitchesBetweenJournalAndSystemWithoutPolling(t *testing.T) {
	// Given a dashboard source and selected server.
	source := &fakeDashboardSource{lines: []string{"line"}}
	m := Model{snapshot: snapshotWithServers("web"), dashboardSource: source}

	// When a unit journal and then the system log are requested.
	journalCmd := m.startDashboardLog(collect.LogSource{Kind: collect.LogJournal, Name: "nginx.service"})
	m, followup := updateModel(t, m, journalCmd())
	if followup != nil {
		t.Fatal("journal snapshot scheduled an automatic refresh")
	}
	systemCmd := m.startDashboardLog(collect.LogSource{Kind: collect.LogSystem})
	m, followup = updateModel(t, m, systemCmd())

	// Then both requests are static, bounded to 50 lines, and the latest source is system.
	if followup != nil || len(source.logRequests) != 2 {
		t.Fatalf("followup=%v requests=%d", followup, len(source.logRequests))
	}
	if source.logRequests[0].Source.Name != "nginx.service" || source.logRequests[1].Source.Kind != collect.LogSystem {
		t.Fatalf("sources = %#v", source.logRequests)
	}
	if !slices.Equal(source.logLines, []int{50, 50}) || m.dashboard.logs.source.Kind != collect.LogSystem {
		t.Fatalf("line limits=%#v source=%#v", source.logLines, m.dashboard.logs.source)
	}
}
