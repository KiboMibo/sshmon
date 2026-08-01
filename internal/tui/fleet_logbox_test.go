package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kibomibo/sshmon/internal/collect"
)

func logboxTestModel() (Model, *fakeLogStreamer) {
	streamer := &fakeLogStreamer{streams: []collect.LogStream{{
		Lines:  make(chan string, 1),
		Errors: make(chan error, 1),
		Close:  func() error { return nil },
	}}}
	m := Model{
		screen:    screenFleet,
		snapshot:  snapshotWithServers("web"),
		logSource: streamer,
		logs:      newLogsScreen(),
		layout:    newLayout(120, 30),
	}
	return m, streamer
}

func TestFleetLogboxOpensInsideListAndEscapeCloses(t *testing.T) {
	// Given: the fleet screen with a server and a log stream available.
	m, streamer := logboxTestModel()

	// When: the log drawer is opened with "l" and the returned command runs.
	opened, cmd := updateModel(t, m, key("l"))
	if cmd != nil {
		cmd()
	}

	// Then: the drawer is visible inside the list and a stream was requested.
	if !opened.fleet.logbox {
		t.Fatal("logbox not opened")
	}
	if view := opened.View(); !strings.Contains(view, "ЛОГИ ·") {
		t.Fatalf("drawer missing from view:\n%s", view)
	}
	if len(streamer.requests) != 1 || streamer.requests[0].Server != "web" {
		t.Fatalf("requests = %#v", streamer.requests)
	}

	// When: escape is pressed.
	closed, _ := updateModel(t, opened, tea.KeyMsg{Type: tea.KeyEsc})

	// Then: the drawer is gone and the fleet screen stays.
	if closed.fleet.logbox {
		t.Fatal("logbox still open")
	}
	if closed.screen != screenFleet {
		t.Fatalf("screen = %v", closed.screen)
	}
}

func TestFleetLogboxSitsRightUnderItsHostRow(t *testing.T) {
	// Given: экран флота с тремя хостами и курсором на среднем.
	streamer := &fakeLogStreamer{streams: []collect.LogStream{{
		Lines:  make(chan string, 1),
		Errors: make(chan error, 1),
		Close:  func() error { return nil },
	}}}
	m := Model{
		screen:    screenFleet,
		snapshot:  snapshotWithServers("web", "db", "cache"),
		logSource: streamer,
		logs:      newLogsScreen(),
		layout:    newLayout(120, 34),
		fleet:     newFleetModel(),
		selected:  1,
	}

	// When: открыт ящик логов.
	opened, cmd := updateModel(t, m, key("l"))
	if cmd != nil {
		cmd()
	}
	lines := strings.Split(opened.View(), "\n")

	// Then: заголовок ящика идёт следующей строкой после строки своего хоста,
	// а строка соседнего хоста — уже после ящика.
	row, box, next := -1, -1, -1
	for i, line := range lines {
		plain := stripANSI(line)
		switch {
		case strings.Contains(plain, fleetMarker+"db"):
			row = i
		case strings.Contains(plain, "ЛОГИ · db"):
			box = i
		case strings.Contains(plain, "cache") && next < 0:
			next = i
		}
	}
	if row < 0 || box < 0 || next < 0 {
		t.Fatalf("строка=%d ящик=%d следующий хост=%d:\n%s", row, box, next, opened.View())
	}
	if box != row+1 {
		t.Fatalf("ящик оторван от своей строки (строка %d, ящик %d):\n%s", row, box, opened.View())
	}
	if next <= box {
		t.Fatalf("список не продолжился под ящиком (ящик %d, cache %d):\n%s", box, next, opened.View())
	}
	// И: ящик врезан в панель списка, а не нарисован над ней.
	if !strings.Contains(stripANSI(lines[box]), "│ ╭─ ЛОГИ · db") {
		t.Fatalf("ящик рисуется вне панели списка: %q", stripANSI(lines[box]))
	}
}

func TestFleetLogboxStaysInFrameWithItsRowOnALongList(t *testing.T) {
	// Given: 28 хостов на невысоком терминале и курсор на последнем.
	servers := make([]string, 0, 28)
	for i := range 28 {
		servers = append(servers, fmt.Sprintf("host-%02d", i))
	}
	streamer := &fakeLogStreamer{streams: []collect.LogStream{{
		Lines:  make(chan string, 1),
		Errors: make(chan error, 1),
		Close:  func() error { return nil },
	}}}
	for _, size := range [][2]int{{120, 24}, {80, 24}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			m := Model{
				screen:    screenFleet,
				snapshot:  snapshotWithServers(servers...),
				logSource: streamer,
				logs:      newLogsScreen(),
				fleet:     newFleetModel(),
				selected:  27,
			}
			m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			opened, _ := updateModel(t, m, key("l"))
			for i := range 6 {
				opened.logs.buffer.Append(fmt.Sprintf("19:41:0%d info строка", i))
			}

			// When: кадр отрисован.
			view := opened.View()

			// Then: в кадре и выделенная строка, и её ящик, и кадр по высоте.
			if lines := strings.Split(view, "\n"); len(lines) != size[1] {
				t.Fatalf("кадр в %d строк при высоте %d:\n%s", len(lines), size[1], view)
			}
			if !strings.Contains(view, "host-27") || !strings.Contains(view, fleetMarker) {
				t.Fatalf("выделенная строка уехала за край:\n%s", view)
			}
			if !strings.Contains(view, "ЛОГИ · host-27") || !strings.Contains(view, "19:41:05") {
				t.Fatalf("ящик выехал за край вместе с логами:\n%s", view)
			}
		})
	}
}

func TestFleetLogboxEnterGoesFullScreen(t *testing.T) {
	// Given: the fleet screen with an open log drawer.
	m, _ := logboxTestModel()
	opened, _ := updateModel(t, m, key("l"))

	// When: enter is pressed inside the drawer.
	full, _ := updateModel(t, opened, tea.KeyMsg{Type: tea.KeyEnter})

	// Then: the full logs screen takes over and the drawer closes.
	if full.screen != screenLogs {
		t.Fatalf("screen = %v", full.screen)
	}
	if full.fleet.logbox {
		t.Fatal("logbox still open")
	}
}

func TestFleetLogboxLeavesGlobalKeysToTheGlobalHandler(t *testing.T) {
	// Given: открытый ящик логов под выбранной строкой списка.
	m, _ := logboxTestModel()
	opened, _ := updateModel(t, m, key("l"))

	// When: нажата «c» — справка и README обещают на ней чат.
	chat, _ := updateModel(t, opened, key("c"))

	// Then: открыт чат, а не «контекст ±5» экрана логов, и ящик на месте.
	if chat.overlay != overlayChat {
		t.Fatalf("overlay = %v", chat.overlay)
	}
	if chat.logs.contextLines != nil {
		t.Fatalf("ящик ушёл в режим контекста: %#v", chat.logs.contextLines)
	}
	if !chat.fleet.logbox {
		t.Fatal("ящик закрылся сам собой")
	}

	// And: «l» повторно закрывает ящик, а не перезапускает поток.
	closed, cmd := updateModel(t, opened, key("l"))
	if closed.fleet.logbox || cmd != nil {
		t.Fatalf("logbox=%v cmd=%v", closed.fleet.logbox, cmd)
	}
}

func TestFleetLogboxMovementSwitchesHostAndStream(t *testing.T) {
	// Given: an open log drawer over a fleet of two hosts.
	streamer := &fakeLogStreamer{streams: []collect.LogStream{
		{Lines: make(chan string, 1), Errors: make(chan error, 1), Close: func() error { return nil }},
		{Lines: make(chan string, 1), Errors: make(chan error, 1), Close: func() error { return nil }},
	}}
	m := Model{
		screen:    screenFleet,
		snapshot:  snapshotWithServers("web", "db"),
		logSource: streamer,
		logs:      newLogsScreen(),
		layout:    newLayout(120, 30),
		fleet:     newFleetModel(),
	}
	opened, cmd := updateModel(t, m, key("l"))
	if cmd != nil {
		cmd()
	}

	// When: the selection moves down inside the drawer and the debounce pause
	// expires — поток перезапускается не на нажатие, а после паузы тишины.
	moved, cmd := updateModel(t, opened, key("j"))
	if cmd == nil {
		t.Fatal("movement inside the drawer produced no command")
	}
	moved, cmd = updateModel(t, moved, debounceMsg{kind: debounceLogs, generation: moved.logs.generation})
	if cmd == nil {
		t.Fatal("дебаунс не запустил поток")
	}
	cmd()

	// Then: both the header and the stream follow the new host.
	if moved.selected != 1 || !moved.fleet.logbox {
		t.Fatalf("selected=%d logbox=%v", moved.selected, moved.fleet.logbox)
	}
	if len(streamer.requests) != 2 || streamer.requests[1].Server != "db" {
		t.Fatalf("requests = %#v", streamer.requests)
	}
	if !strings.Contains(moved.View(), "ЛОГИ · db") {
		t.Fatalf("drawer header kept the old host:\n%s", moved.View())
	}
}

func TestFleetLogboxHostSwitchDropsPreviousHostLines(t *testing.T) {
	// Given: an open log drawer with lines already collected from the first host.
	streamer := &fakeLogStreamer{streams: []collect.LogStream{
		{Lines: make(chan string, 1), Errors: make(chan error, 1), Close: func() error { return nil }},
		{Lines: make(chan string, 1), Errors: make(chan error, 1), Close: func() error { return nil }},
	}}
	m := Model{
		screen:    screenFleet,
		snapshot:  snapshotWithServers("web", "db"),
		logSource: streamer,
		logs:      newLogsScreen(),
		layout:    newLayout(120, 30),
		fleet:     newFleetModel(),
	}
	opened, _ := updateModel(t, m, key("l"))
	opened.logs.buffer.Append("web-only line")

	// When: the selection moves to the next host.
	moved, _ := updateModel(t, opened, key("j"))

	// Then: the drawer body is empty instead of showing the previous host's lines.
	if got := moved.logs.buffer.Visible(); len(got) != 0 {
		t.Fatalf("visible after host switch = %#v", got)
	}
	if view := moved.View(); strings.Contains(view, "web-only line") {
		t.Fatalf("drawer kept lines of the previous host:\n%s", view)
	}
}

func TestFleetLogboxCountsLinesArrivedSinceLastLook(t *testing.T) {
	// Given: an open log drawer on a filtered level.
	m, _ := logboxTestModel()
	opened, cmd := updateModel(t, m, key("l"))
	if cmd != nil {
		cmd()
	}

	// When: three lines arrive after the drawer was opened.
	for _, line := range []string{"info started", "WARN disk almost full", "ERROR failed to write"} {
		opened.logs.buffer.Append(line)
	}

	// Then: the counter reports them as new next to the visible/total pair.
	if hint := opened.fleetLogboxCount(); hint != "3 новых · 3 из 3" {
		t.Fatalf("hint = %q", hint)
	}
	if view := opened.View(); !strings.Contains(view, "3 новых") {
		t.Fatalf("drawer misses the new-lines counter:\n%s", view)
	}

	// When: the user scrolls back to the tail.
	seen, _ := updateModel(t, opened, tea.KeyMsg{Type: tea.KeyEnd})

	// Then: nothing is new any more until the next line arrives.
	if hint := seen.fleetLogboxCount(); hint != "3 из 3" {
		t.Fatalf("hint after end = %q", hint)
	}
	seen.logs.buffer.Append("info one more")
	if hint := seen.fleetLogboxCount(); hint != "1 новых · 4 из 4" {
		t.Fatalf("hint after a fresh line = %q", hint)
	}
}

func TestFleetLogboxStatusFollowsLayoutWording(t *testing.T) {
	// Given: a drawer streaming a journal unit filtered from warnings up.
	m, _ := logboxTestModel()
	m.logs.sources = []collect.LogSource{{Kind: collect.LogJournal, Name: "postgres"}}
	m.logs.level = logLevelWarn

	// When: the drawer status line is built.
	status := m.fleetLogboxStatus()

	// Then: the source is short and the level reads as a threshold.
	if !strings.HasPrefix(status, "postgres · warn+ · ") {
		t.Fatalf("status = %q", status)
	}
}

func TestLogsLevelAxisFiltersAndCounts(t *testing.T) {
	// Given: a logs screen holding lines of mixed severity.
	logs := newLogsScreen()
	logs.ensure()
	logs.buffer.Append("info started")
	logs.buffer.Append("WARN disk almost full")
	logs.buffer.Append("ERROR failed to write")

	// When: the level axis is switched to warn and above.
	logs.level = logLevelWarn
	visible := logs.visibleLines()

	// Then: only warn and error survive, and the counter reports both totals.
	if len(visible) != 2 {
		t.Fatalf("visible = %#v", visible)
	}
	m := Model{logs: logs}
	if hint := m.logsCountHint(); hint != "2 из 3 строк" {
		t.Fatalf("hint = %q", hint)
	}
}

func TestLogSourceLabelKeepsUnitName(t *testing.T) {
	// Given: a system source and a journal unit source.
	// When: both are labelled for the source axis.
	// Then: the unit name is kept and the plain system source stays short.
	if label := logSourceLabel(collect.LogSource{Kind: collect.LogSystem}); label != "system" {
		t.Fatalf("label = %q", label)
	}
	if label := logSourceLabel(collect.LogSource{Kind: collect.LogJournal, Name: "sshd.service"}); label != "journal/sshd.service" {
		t.Fatalf("label = %q", label)
	}
}
