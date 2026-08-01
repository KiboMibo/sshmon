package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/kibomibo/sshmon/internal/collect"
)

// TestContainerUptimeCoversEveryDockerForm — Дано: все формы, которые отдаёт
// `{{.RunningFor}}`; Когда: их приводит к колонке аптайма TUI; Тогда: получаем
// компактный русский вид и ни одной английской строки.
func TestContainerUptimeCoversEveryDockerForm(t *testing.T) {
	for _, tc := range []struct{ runningFor, want string }{
		{"Less than a second", "0с"},
		{"1 second ago", "1с"},
		{"14 seconds ago", "14с"},
		{"About a minute ago", "1м"},
		{"12 minutes ago", "12м"},
		{"About an hour ago", "1ч"},
		{"3 hours ago", "3ч"},
		{"2 days ago", "2д"},
		{"20 weeks ago", "140д"},
		{"4 months ago", "120д"},
		{"2 years ago", "730д"},
		{"3 weeks", "21д"},
		{"", ""},
		{"недавно", ""},
	} {
		if got := containerUptime(tc.runningFor); got != tc.want {
			t.Fatalf("containerUptime(%q) = %q, want %q", tc.runningFor, got, tc.want)
		}
	}
}

// TestCompactDurationKeepsOneUnit — Дано: длительности разных порядков;
// Когда: они попадают в колонку; Тогда: остаётся одна значащая единица.
func TestCompactDurationKeepsOneUnit(t *testing.T) {
	for _, tc := range []struct {
		value time.Duration
		want  string
	}{
		{140 * 24 * time.Hour, "140д"},
		{25 * time.Hour, "1д"},
		{3*time.Hour + 40*time.Minute, "3ч"},
		{12 * time.Minute, "12м"},
		{45 * time.Second, "45с"},
		{0, "0с"},
	} {
		if got := compactDuration(tc.value); got != tc.want {
			t.Fatalf("compactDuration(%v) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

// TestContainerStatusIsHumanReadable — Дано: сырые статусы docker'а;
// Когда: их читает оператор; Тогда: остаётся состояние и код выхода.
func TestContainerStatusIsHumanReadable(t *testing.T) {
	for _, tc := range []struct{ status, want string }{
		{"Up 20 weeks", "up"},
		{"Up 2 hours (unhealthy)", "up (unhealthy)"},
		{"Up 2 hours (healthy)", "up"},
		{"Exited (0) 12 days ago", "exited (0)"},
		{"Exited (137) 5 minutes ago", "exited (137)"},
		{"Restarting (1) 3 seconds ago", "restarting (1)"},
		{"Created", "created"},
		{"", "—"},
	} {
		if got := containerStatus(tc.status); got != tc.want {
			t.Fatalf("containerStatus(%q) = %q, want %q", tc.status, got, tc.want)
		}
	}
}

// TestContainerMemoryShowsUsedPartOnly — Дано: `{{.MemUsage}}` docker'а;
// Когда: значение идёт в узкую колонку; Тогда: остаётся использованная часть.
func TestContainerMemoryShowsUsedPartOnly(t *testing.T) {
	for _, tc := range []struct{ usage, want string }{
		{"6.1GiB / 15.6GiB", "6.1G"},
		{"120MiB / 512MiB", "120M"},
		{"340kB / 1GB", "340K"},
		{"-- / --", ""},
		{"", ""},
	} {
		if got := containerMemory(tc.usage); got != tc.want {
			t.Fatalf("containerMemory(%q) = %q, want %q", tc.usage, got, tc.want)
		}
	}
}

// TestContainerCountsCompactMatchesMockup — Дано: контейнеры в трёх
// состояниях; Когда: счётчики уходят в заголовок плитки; Тогда: «●2 ○1 ⚠1».
func TestContainerCountsCompactMatchesMockup(t *testing.T) {
	got := containerCountsCompact([]collect.Container{
		{Status: "Up 3 days"}, {Status: "Up 2 hours"},
		{Status: "Exited (0) 1 day ago"},
		{Status: "Restarting (1) 2 seconds ago"},
	})
	if !strings.Contains(got, "●2") || !strings.Contains(got, "○1") || !strings.Contains(got, "⚠1") {
		t.Fatalf("containerCountsCompact = %q", got)
	}
}

func TestSortContainersUsesNameAsStableTieBreaker(t *testing.T) {
	// Given containers with equal CPU usage.
	items := []collect.Container{{Name: "worker", Image: "app:v1", Status: "Up", CPUPct: 10}, {Name: "api", Image: "app:v2", Status: "Up", CPUPct: 10}}

	// When sorted by CPU descending.
	got := sortContainers(items, containerSortCPU)

	// Then the name provides deterministic ordering for equal values.
	if got[0].Name != "api" || got[1].Name != "worker" {
		t.Fatalf("unexpected order: %+v", got)
	}
}

func TestRenderContainersIsObservationOnly(t *testing.T) {
	// Given a ready container screen.
	m := Model{screen: screenContainers, snapshot: snapshotWithServers("web"), layout: newLayout(100, 24)}
	m.containers.status = diagnosticsReady
	m.containers.items = []collect.Container{{Name: "api", Image: "app:v2", Status: "Up 2h", CPUPct: 4, MemPct: 12}}

	// When rendered.
	view := m.containers.view(m.screenContext())

	// Then state and resource columns are present and no mutation hint exists.
	for _, want := range []string{"ИМЯ", "ОБРАЗ", "СТАТУС", "CPU", "MEM", "api", "Up 2h"} {
		if !containsText(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
	for _, forbidden := range []string{"start", "stop", "restart", "exec"} {
		if containsText(view, forbidden) {
			t.Fatalf("read-only view contains %q", forbidden)
		}
	}
}
