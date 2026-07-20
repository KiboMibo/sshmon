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

	// Then: полный хвост ошибки виден (без обрезки), а не многоточие.
	if !strings.Contains(view, "переподключитесь") {
		t.Fatalf("error text was truncated, view:\n%s", view)
	}
	if strings.Contains(view, "…") {
		// Допустимо только если есть отдельная обрезанная строка, но ошибка должна быть цельной.
		t.Fatalf("view contains ellipsis (truncation), but error should wrap:\n%s", view)
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
