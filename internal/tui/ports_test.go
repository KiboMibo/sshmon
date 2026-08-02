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
	view := m.ports.view(m.screenContext())

	// Then protocol, local address, process and PID are visible.
	for _, want := range []string{"PROTO", "LOCAL", "ПРОЦЕСС", "PID", "sshd", "100"} {
		if !containsText(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

// TestPortsExplainMissingProcessNames — Дано: порты, собранные без root'а (ss
// не назвал ни одного владельца сокета); Когда: рисуются экран портов и плитка
// ПОРТЫ экрана сервера; Тогда: оба объясняют пустую колонку правами, а не
// молчат. И: как только хоть один процесс назвался, подсказка уходит.
func TestPortsExplainMissingProcessNames(t *testing.T) {
	anonymous := []collect.Port{
		{Proto: "tcp", Local: "0.0.0.0:443"},
		{Proto: "tcp", Local: "127.0.0.1:8009"},
	}

	// Тогда: подсказка появляется только там, где имён нет ни у кого.
	for _, tc := range []struct {
		name  string
		ports []collect.Port
		want  bool
	}{
		{"портов нет", nil, false},
		{"имён нет ни у кого", anonymous, true},
		{"хотя бы одно имя есть", append(append([]collect.Port(nil), anonymous...), collect.Port{Proto: "tcp", Local: ":22", Process: "sshd", PID: 1}), false},
	} {
		if got := portsRootHint(tc.ports) != ""; got != tc.want {
			t.Fatalf("%s: подсказка = %v, ожидалось %v", tc.name, got, tc.want)
		}
	}

	// Когда: рисуется полноэкранный список портов.
	m := Model{screen: screenPorts, snapshot: snapshotWithServers("web"), layout: newLayout(100, 24)}
	m.ports.status = diagnosticsReady
	m.ports.items = anonymous
	if view := m.ports.view(m.screenContext()); !containsText(view, "только под root") {
		t.Fatalf("экран портов не объяснил пустую колонку процесса:\n%s", view)
	}

	// Когда: рисуется плитка ПОРТЫ экрана сервера.
	dash := dashboardWorkspaceFixture()
	dash.snapshot.Servers[0].Ports = anonymous
	if view := stripANSI(dash.View()); !containsText(view, "только под root") {
		t.Fatalf("плитка ПОРТЫ не объяснила пустую колонку процесса:\n%s", view)
	}
	// И: подсказка стоит первой строкой плитки, до самих записей.
	lines := m.serverPortTileLines(dash.snapshot.Servers[0], 40)
	if len(lines) < 2 || !containsText(lines[0], "только под root") || !containsText(lines[1], "0.0.0.0:443") {
		t.Fatalf("подсказка не первой строкой плитки: %#v", lines)
	}
}
