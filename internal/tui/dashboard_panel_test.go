package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/kibomibo/sshmon/internal/collect"
)

func TestPanelBoxDrawsTitledTopAndHintedBottom(t *testing.T) {
	// Given a panel title, a contextual hint, and one content row.
	// When the content is boxed at a fixed width.
	lines := panelBox("CPU", "p процессы", 30, []string{"load 0.4"})

	// Then the box wraps the row with a titled top, a hinted bottom, and side borders of uniform width.
	if len(lines) != 3 {
		t.Fatalf("want 3 lines, got %d: %#v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "╭") || !strings.Contains(lines[0], "╮") || !strings.Contains(lines[0], "CPU") {
		t.Fatalf("top border missing corners/title: %q", lines[0])
	}
	if !strings.Contains(lines[2], "╰") || !strings.Contains(lines[2], "╯") || !strings.Contains(lines[2], "p процессы") {
		t.Fatalf("bottom border missing corners/hint: %q", lines[2])
	}
	if strings.Count(lines[1], "│") != 2 || !strings.Contains(lines[1], "load 0.4") {
		t.Fatalf("content row missing side borders: %q", lines[1])
	}
	for i, line := range lines {
		if width := lipgloss.Width(line); width != 30 {
			t.Fatalf("line %d width = %d, want 30: %q", i, width, line)
		}
	}
}

func TestWrapWordsBreaksLongTextToFitWidth(t *testing.T) {
	// Given: длинная строка с русским текстом, явно превышающая целевую ширину.
	long := "host-key сервера не совпадает с записью в known_hosts — выполните ssh-keygen -R и переподключитесь"

	// When: wrapWords сворачивает её по словам под ширину 40.
	lines := wrapWords(long, 40)

	// Then: каждая строка укладывается в ширину, строк больше одной, и ничего не потеряно.
	if len(lines) < 2 {
		t.Fatalf("expected multiple wrapped lines, got %d: %v", len(lines), lines)
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w > 40 {
			t.Fatalf("line %d width=%d > 40: %q", i, w, line)
		}
	}
	joined := strings.Join(lines, " ")
	for _, want := range []string{"host-key", "ssh-keygen", "переподключитесь"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("wrapped output lost %q: %v", want, lines)
		}
	}
}

func TestDashboardErrorRendersAsBorderedParagraphWithoutTruncation(t *testing.T) {
	// Given: сервер с длинной ошибкой SSH, которая раньше обрезалась через fitLine.
	m := dashboardWorkspaceFixture()
	m.layout = newLayout(120, 30)
	server := m.snapshot.Servers[0]
	server.Online = false
	server.Err = "host-key сервера не совпадает с записью в known_hosts — выполните ssh-keygen -R и переподключитесь"
	m.snapshot.Servers[0] = server

	// When: Dashboard рендерится.
	view := m.View()

	// Then: полный хвост ошибки виден (без обрезки) — значит, текст перенёсся, а не обрезался.
	if !strings.Contains(view, "переподключитесь") {
		t.Fatalf("error text was truncated, view:\n%s", view)
	}
}

func TestDashboardWideDrawsBorderedPanelsWithLocalHints(t *testing.T) {
	// Given a wide dashboard with metrics, running Docker, systemd units, and logs.
	m := dashboardWorkspaceFixture()
	m.layout = newLayout(120, 30)
	m.dashboard.containers.items = []collect.Container{{Name: "api", Status: "Up", CPUPct: 3, MemPct: 4}}

	// When the dashboard is rendered on a wide terminal.
	view := m.View()

	// Then every panel is framed and carries its own data-local hint in the border.
	for _, want := range []string{
		"╭", "╮", "╰", "╯",
		"p процессы · o порты · h история",
		"d контейнеры",
		"f фильтр · j/k · enter journal",
		"l логи · x системный лог",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("wide dashboard missing %q:\n%s", want, view)
		}
	}
}

func TestDashboardWideHasNoNoDataFiller(t *testing.T) {
	t.Parallel()
	// Given a wide dashboard where SYSTEMD has many units but МЕТРИКИ is short.
	m := dashboardWorkspaceFixture()
	m.layout = newLayout(160, 50)
	units := make([]collect.SystemdUnit, 25)
	for i := range units {
		units[i] = collect.SystemdUnit{Name: "svc" + string(rune('a'+i)) + ".service", Active: "active", Sub: "running"}
	}
	m.dashboard.units = dashboardUnitsState{items: units, status: diagnosticsReady}
	// When the full view is rendered.
	view := m.View()
	// Then no NO DATA filler leaks into any panel — short cells use blank padding.
	if strings.Contains(view, "NO DATA") {
		t.Fatalf("view still contains NO DATA filler:\n%s", view)
	}
}

func TestFitPanelHeightPadsAndScrolls(t *testing.T) {
	t.Parallel()
	// Given content shorter than the target height.
	short := fitPanelHeight([]string{"a", "b"}, 5, 0)
	// Then it is blank-padded to exactly the height with no NO DATA.
	if len(short) != 5 {
		t.Fatalf("short height=%d want 5", len(short))
	}
	if short[2] != "" || short[4] != "" {
		t.Fatalf("padding is not blank: %#v", short)
	}
	// Given content taller than the height with a scroll offset.
	long := fitPanelHeight([]string{"1", "2", "3", "4", "5"}, 2, 1)
	// Then the window is exactly height rows, offset from the top.
	if len(long) != 2 || long[0] != "2" || long[1] != "3" {
		t.Fatalf("scroll window wrong: %#v", long)
	}
}

func TestContainerStatusDotDerivesFromStatus(t *testing.T) {
	t.Parallel()
	// Given container statuses in various states.
	// When the status dot is derived.
	up := containerStatusDot("Up 2 hours")
	exited := containerStatusDot("Exited (0) 5 min ago")
	paused := containerStatusDot("Paused")
	// Then every status emits the dot glyph.
	for _, dot := range []string{up, exited, paused} {
		if !strings.Contains(dot, "●") {
			t.Fatalf("dot glyph missing: %q", dot)
		}
	}
}

func TestUnitStateTextColorsByActiveSub(t *testing.T) {
	t.Parallel()
	// Given systemd unit active/sub combinations.
	// When the state text is derived.
	running := unitStateText("active", "running")
	failed := unitStateText("failed", "failed")
	inactive := unitStateText("inactive", "dead")
	activating := unitStateText("activating", "start-pre")
	// Then each retains the state words for identification.
	if !strings.Contains(running, "running") {
		t.Fatalf("running text wrong: %q", running)
	}
	if !strings.Contains(failed, "failed") {
		t.Fatalf("failed text wrong: %q", failed)
	}
	if !strings.Contains(inactive, "dead") {
		t.Fatalf("inactive text wrong: %q", inactive)
	}
	if !strings.Contains(activating, "start-pre") {
		t.Fatalf("activating text wrong: %q", activating)
	}
}

func TestDockerContentShowsStatusDotAndPorts(t *testing.T) {
	t.Parallel()
	// Given a dashboard with one running container exposing ports.
	m := dashboardWorkspaceFixture()
	m.dashboard.containers = dashboardContainersState{
		items:  []collect.Container{{Name: "api", Status: "Up 2 hours", Ports: "0.0.0.0:8080->80/tcp", CPUPct: 3, MemPct: 4}},
		status: diagnosticsReady,
	}
	// When docker content is rendered.
	content := m.dashboardDockerContent()
	// Then it shows a status dot, the container name, and the ports string.
	joined := strings.Join(content, "\n")
	if !strings.Contains(joined, "●") {
		t.Fatalf("missing status dot: %s", joined)
	}
	if !strings.Contains(joined, "api") {
		t.Fatalf("missing container name: %s", joined)
	}
	if !strings.Contains(joined, "8080") {
		t.Fatalf("missing ports: %s", joined)
	}
}

func TestSystemdContentColorsStateText(t *testing.T) {
	t.Parallel()
	// Given a dashboard with units in various states.
	m := dashboardWorkspaceFixture()
	m.dashboard.units = dashboardUnitsState{
		items: []collect.SystemdUnit{
			{Name: "sshd.service", Active: "active", Sub: "running"},
			{Name: "fail.service", Active: "failed", Sub: "failed"},
		},
		status: diagnosticsReady,
	}
	// When systemd content is rendered.
	content := m.dashboardUnitsContent()
	// Then both unit names and state texts appear.
	joined := strings.Join(content, "\n")
	if !strings.Contains(joined, "sshd.service") || !strings.Contains(joined, "running") {
		t.Fatalf("missing running unit: %s", joined)
	}
	if !strings.Contains(joined, "fail.service") || !strings.Contains(joined, "failed") {
		t.Fatalf("missing failed unit: %s", joined)
	}
}

func TestDashboardWideRowOneHasThreeColumnsDockerBelow(t *testing.T) {
	t.Parallel()
	// Given a wide dashboard with running Docker.
	m := dashboardWorkspaceFixture()
	m.layout = newLayout(120, 30)
	m.dashboard.containers = dashboardContainersState{items: []collect.Container{{Name: "api", Status: "Up"}}, status: diagnosticsReady}
	// When the full view is rendered.
	view := m.View()
	// Then МЕТРИКИ, СЕТЬ, SYSTEMD share row 1 and DOCKER sits on a later row.
	metricsLine, netLine, systemdLine, dockerLine := -1, -1, -1, -1
	for i, line := range strings.Split(view, "\n") {
		if !strings.Contains(line, "╭─") {
			continue
		}
		if strings.Contains(line, "МЕТРИКИ") {
			metricsLine = i
		}
		if strings.Contains(line, "СЕТЬ") {
			netLine = i
		}
		if strings.Contains(line, "SYSTEMD") {
			systemdLine = i
		}
		if strings.Contains(line, "DOCKER") {
			dockerLine = i
		}
	}
	if metricsLine < 0 || netLine != metricsLine || systemdLine != metricsLine {
		t.Fatalf("row 1 misaligned: МЕТРИКИ=%d СЕТЬ=%d SYSTEMD=%d", metricsLine, netLine, systemdLine)
	}
	if dockerLine <= metricsLine {
		t.Fatalf("DOCKER=%d must be below row 1=%d", dockerLine, metricsLine)
	}
}
