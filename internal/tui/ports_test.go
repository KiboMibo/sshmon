package tui

import (
	"testing"

	"github.com/kibomibo/sshmon/internal/collect"
)

func TestSortPortsOrdersProtocolLocalProcessAndPID(t *testing.T) {
	// Given ports deliberately out of display order.
	items := []collect.Port{{Proto: "udp", Local: ":53", Process: "dns", PID: 9}, {Proto: "tcp", Local: ":443", Process: "web", PID: 7}, {Proto: "tcp", Local: ":22", Process: "sshd", PID: 2}}

	// When sorted by the default local-address order.
	got := sortPorts(items, portSortLocal)

	// Then stable protocol/local/process/PID keys define the result.
	if got[0].Local != ":22" || got[1].Local != ":443" || got[2].Local != ":53" {
		t.Fatalf("unexpected order: %+v", got)
	}
}

func TestRenderPortsShowsProcessAndPID(t *testing.T) {
	// Given a ready ports screen.
	m := Model{screen: screenPorts, snapshot: snapshotWithServers("web"), layout: newLayout(100, 24)}
	m.ports.status = diagnosticsReady
	m.ports.items = []collect.Port{{Proto: "tcp", Local: "0.0.0.0:22", Process: "sshd", PID: 100}}

	// When rendered.
	view := m.renderPorts()

	// Then protocol, local address, process and PID are visible.
	for _, want := range []string{"PROTO", "LOCAL", "ПРОЦЕСС", "PID", "sshd", "100"} {
		if !containsText(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}
