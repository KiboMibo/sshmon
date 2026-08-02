package tui

import (
	"errors"
	"fmt"
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

func TestDashboardProblemsPanelWrapsFullIssueWithoutTruncation(t *testing.T) {
	// Given: сервер с длинной проблемой, которая раньше обрезалась через fitLine.
	m := dashboardWorkspaceFixture()
	m.layout = newLayout(120, 30)
	server := m.snapshot.Servers[0]
	server.Online = false
	m.snapshot.Servers[0] = server
	m.snapshot.Issues = []collect.Issue{{
		Server:   server.Name,
		Severity: "crit",
		Msg:      "недоступен: host-key сервера не совпадает с записью в known_hosts — выполните ssh-keygen -R и переподключитесь",
	}}

	// When: Dashboard рендерится.
	view := m.View()

	// Then: полный хвост проблемы виден в единственной панели ПРОБЛЕМЫ — без обрезки и без дубля ОШИБКА SSH.
	if !strings.Contains(view, "переподключитесь") {
		t.Fatalf("issue text was truncated, view:\n%s", view)
	}
	if strings.Contains(view, "ОШИБКА SSH") {
		t.Fatalf("duplicate ОШИБКА SSH panel still present, view:\n%s", view)
	}
}

func TestServerScreenDrawsBorderedPanelsWithLocalHints(t *testing.T) {
	// Given a server screen with metrics, running Docker, systemd units, and logs.
	m := serverScreenModel(120, 30)

	// When the screen is rendered.
	view := m.View()

	// Then every panel is framed and carries its own data-local hint.
	for _, want := range []string{
		"╭", "╮", "╰", "╯",
		"d контейнеры",
		"f фильтр · j/k · enter journal",
		"o порты",
		"l логи · s источник",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("server screen missing %q:\n%s", want, view)
		}
	}
}

func TestServerScreenHasNoNoDataFiller(t *testing.T) {
	t.Parallel()
	// Given a server screen where СЕРВИСЫ has many units but the grid is short.
	m := serverScreenModel(160, 50)
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

func TestContainerStatusDotSeparatesRunningStoppedAndBroken(t *testing.T) {
	t.Parallel()
	// Given container statuses in various states.
	// When the status dot is derived.
	// Then each group gets its own glyph, различимый и без цвета.
	for _, tc := range []struct{ status, want string }{
		{"Up 2 hours", "●"},
		{"Exited (0) 5 min ago", "○"},
		{"Created", "○"},
		{"Restarting (1) 3 seconds ago", "⚠"},
		{"Paused", "⚠"},
	} {
		if got := containerStatusDot(tc.status); !strings.Contains(got, tc.want) {
			t.Fatalf("containerStatusDot(%q) = %q, want %q", tc.status, got, tc.want)
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

func TestDockerContentShowsStateUptimeAndMemory(t *testing.T) {
	t.Parallel()
	// Given a dashboard with a running and a stopped container.
	m := dashboardWorkspaceFixture()
	m.dashboard.containers = dashboardContainersState{status: diagnosticsReady, items: []collect.Container{
		{Name: "pg-main", Status: "Up 20 weeks", RunningFor: "20 weeks ago", MemUsage: "6.1GiB / 15.6GiB", Ports: "0.0.0.0:5432->5432/tcp", CPUPct: 3, MemPct: 4},
		{Name: "mailhog", Status: "Exited (0) 12 days ago", RunningFor: "12 days ago", MemUsage: "0B / 0B"},
	}}

	// When docker content is rendered.
	joined := strings.Join(m.dashboardDockerContent(45), "\n")

	// Then each row is «имя · статус · аптайм · память» без процентов и портов.
	for _, want := range []string{"● ", "pg-main", "up", "140д", "6.1G", "○ ", "mailhog", "exited (0)", "12д"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("docker content missing %q:\n%s", want, joined)
		}
	}
	for _, forbidden := range []string{"%", "5432->5432"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("docker content still shows %q:\n%s", forbidden, joined)
		}
	}
}

// TestDockerTileExplainsWhyListIsEmpty — Дано: состояния запроса контейнеров,
// в которых списка нет; Когда: рисуется содержимое плитки DOCKER; Тогда: она
// называет причину, а не молчит и не исчезает.
func TestDockerTileExplainsWhyListIsEmpty(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		state dashboardContainersState
		want  string
	}{
		{name: "idle", state: dashboardContainersState{}, want: "загрузка…"},
		{name: "loading", state: dashboardContainersState{status: diagnosticsLoading}, want: "загрузка…"},
		{name: "ready", state: dashboardContainersState{status: diagnosticsReady}, want: "контейнеров нет"},
		{
			name:  "unsupported",
			state: dashboardContainersState{status: diagnosticsUnsupported, err: collect.ErrUnsupported},
			want:  "docker не установлен",
		},
		{
			name:  "denied",
			state: dashboardContainersState{status: diagnosticsError, err: fmt.Errorf("%w: permission denied", collect.ErrAccessDenied)},
			want:  "нет доступа к docker",
		},
		{
			name:  "error",
			state: dashboardContainersState{status: diagnosticsError, err: errors.New("dial timeout")},
			want:  "ошибка: dial timeout",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := dashboardWorkspaceFixture()
			m.dashboard.containers = tc.state

			rows := m.dashboardDockerContent(45)

			if len(rows) != 1 || !strings.Contains(rows[0], tc.want) {
				t.Fatalf("содержимое плитки %#v, ожидалось %q", rows, tc.want)
			}
		})
	}
}

// TestServerScreenKeepsDockerTileWithoutContainers — Дано: хост, на котором
// `docker ps` отказал по правам; Когда: экран сервера собран на всех
// поддерживаемых размерах; Тогда: блок DOCKER на месте, кадр не разъехался, а
// причина видна везде, где плитка вообще рисуется рамкой.
func TestServerScreenKeepsDockerTileWithoutContainers(t *testing.T) {
	t.Parallel()
	for _, size := range []struct{ width, height int }{
		{width: 60, height: 16}, {width: 80, height: 24}, {width: 100, height: 16},
		{width: 100, height: 20}, {width: 120, height: 30}, {width: 160, height: 50},
	} {
		m := serverScreenModel(size.width, size.height)
		m.dashboard.containers = dashboardContainersState{
			status: diagnosticsError,
			err:    fmt.Errorf("%w: permission denied while trying to connect to the Docker daemon socket", collect.ErrAccessDenied),
		}

		view := m.View()
		lines := strings.Split(view, "\n")

		if len(lines) != size.height {
			t.Fatalf("%dx%d: %d строк, ожидалось %d:\n%s", size.width, size.height, len(lines), size.height, view)
		}
		for index, line := range lines {
			if got := lipgloss.Width(line); got > size.width {
				t.Fatalf("%dx%d: строка %d шириной %d:\n%q", size.width, size.height, index, got, line)
			}
		}
		for _, want := range []string{"DOCKER", "СЕРВИСЫ", "ЛОГИ"} {
			if !strings.Contains(view, want) {
				t.Fatalf("%dx%d: экран без блока %q:\n%s", size.width, size.height, want, view)
			}
		}
		// Тогда: в рамочной раскладке видно и причину; в аварийной (60×16)
		// от плитки остаётся один заголовок, и объяснению места уже нет.
		if strings.Contains(view, "╭─ DOCKER") && !strings.Contains(view, "нет доступа к docker") {
			t.Fatalf("%dx%d: плитка DOCKER не объяснила состояние:\n%s", size.width, size.height, view)
		}
	}
}

// TestFleetCardAndDockerTileAgreeOnAccessDenied — Дано: тот же отказ по правам
// и карточка флота того же хоста; Когда: обе поверхности отрисованы; Тогда:
// они называют состояние одинаково, а не «контейнеров нет» против «нет доступа».
func TestFleetCardAndDockerTileAgreeOnAccessDenied(t *testing.T) {
	t.Parallel()
	m := dashboardWorkspaceFixture()
	m.dashboard.server = m.snapshot.Servers[0].Name
	m.dashboard.containers = dashboardContainersState{
		status: diagnosticsError,
		err:    fmt.Errorf("%w: permission denied", collect.ErrAccessDenied),
	}

	card := strings.Join(m.fleetCardLines(m.snapshot.Servers[0], 80), "\n")
	tile := strings.Join(m.dashboardDockerContent(45), "\n")

	if !strings.Contains(card, "нет доступа к docker") {
		t.Fatalf("карточка флота молчит о причине:\n%s", card)
	}
	if !strings.Contains(tile, "нет доступа к docker") {
		t.Fatalf("плитка молчит о причине:\n%s", tile)
	}
	// И: про чужой хост диагностика ничего не говорит — там остаётся факт из
	// сэмпла, а он различает «docker не ответил» и «контейнеров нет».
	m.dashboard.server = "other"
	other := strings.Join(m.fleetCardLines(m.snapshot.Servers[0], 80), "\n")
	if strings.Contains(other, "нет доступа к docker") {
		t.Fatalf("состояние чужого хоста утекло в карточку:\n%s", other)
	}
	if !strings.Contains(other, "docker недоступен") {
		t.Fatalf("карточка не назвала docker недоступным:\n%s", other)
	}
	server := m.snapshot.Servers[0]
	server.Docker = collect.DockerCounts{Known: true}
	if empty := strings.Join(m.fleetCardLines(server, 80), "\n"); !strings.Contains(empty, "контейнеров нет") {
		t.Fatalf("живой docker без контейнеров назван недоступным:\n%s", empty)
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

func TestServerScreenDockerAndServicesShareRowLogsBelow(t *testing.T) {
	t.Parallel()
	// Given a server screen wide enough for two columns.
	m := serverScreenModel(120, 30)
	// When the full view is rendered.
	view := m.View()
	// Then DOCKER and СЕРВИСЫ share one row and ЛОГИ sits below them.
	dockerLine, servicesLine, logsLine := -1, -1, -1
	for i, line := range strings.Split(view, "\n") {
		if !strings.Contains(line, "╭─") {
			continue
		}
		if strings.Contains(line, "DOCKER") {
			dockerLine = i
		}
		if strings.Contains(line, "СЕРВИСЫ") {
			servicesLine = i
		}
		if strings.Contains(line, "ЛОГИ") {
			logsLine = i
		}
	}
	if dockerLine < 0 || servicesLine != dockerLine {
		t.Fatalf("row misaligned: DOCKER=%d СЕРВИСЫ=%d\n%s", dockerLine, servicesLine, view)
	}
	if logsLine <= dockerLine {
		t.Fatalf("ЛОГИ=%d must be below row=%d\n%s", logsLine, dockerLine, view)
	}
}

func TestServerScreenStacksPanelsOnEightyColumns(t *testing.T) {
	t.Parallel()
	// Given the same screen on eighty columns.
	m := serverScreenModel(80, 24)

	// When the view is rendered.
	view := m.View()

	// Then the same blocks stack in one column in the mockup order.
	previous := -1
	for _, section := range []string{"DOCKER", "СЕРВИСЫ", "ПОРТЫ", "ЛОГИ"} {
		position := strings.Index(view, section)
		if position <= previous {
			t.Fatalf("section %q is missing or out of order:\n%s", section, view)
		}
		previous = position
	}
}
