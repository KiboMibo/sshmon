package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kibomibo/sshmon/internal/collect"
)

type fakeLogStreamer struct {
	requests []collect.LogRequest
	streams  []collect.LogStream
}

func (f *fakeLogStreamer) StreamLogs(_ context.Context, request collect.LogRequest) (collect.LogStream, error) {
	f.requests = append(f.requests, request)
	if len(f.streams) == 0 {
		return collect.LogStream{}, errors.New("no fake stream")
	}
	stream := f.streams[0]
	f.streams = f.streams[1:]
	return stream, nil
}

func TestLogsOpenStartsStreamAndIgnoresStaleLines(t *testing.T) {
	// Given: a dashboard with one server and a controllable log stream.
	lines := make(chan string, 1)
	errs := make(chan error, 1)
	streamer := &fakeLogStreamer{streams: []collect.LogStream{{Lines: lines, Errors: errs, Close: func() error { return nil }}}}
	m := Model{
		screen:    screenDashboard,
		snapshot:  snapshotWithServers("web"),
		logSource: streamer,
		logs:      newLogsScreen(),
	}

	// When: the logs screen is opened (ctrl+l) and its first stream line arrives.
	updated, openCmd := updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlL})
	opened := openCmd().(logsOpenedMsg)
	updated, waitCmd := updateModel(t, updated, opened)
	lines <- "fresh"
	lineMsg := waitCmd().(logLineMsg)
	updated, _ = updateModel(t, updated, lineMsg)
	updated, _ = updateModel(t, updated, logLineMsg{generation: opened.generation - 1, line: "stale"})

	// Then: one request was started and only the matching generation is visible.
	if len(streamer.requests) != 1 || streamer.requests[0].Server != "web" {
		t.Fatalf("requests = %#v", streamer.requests)
	}
	visible := updated.logs.buffer.Visible()
	if len(visible) != 1 || visible[0] != "fresh" {
		t.Fatalf("visible = %#v", visible)
	}
}

func TestLogsControlsPauseFilterCycleReconnectAndCancel(t *testing.T) {
	// Given: an active logs screen with a second source discovered on the server.
	cancelled := 0
	m := Model{
		screen:   screenLogs,
		snapshot: snapshotWithServers("web"),
		logs:     newLogsScreen(),
	}
	m.dashboard.server = "web"
	m.dashboard.units.items = []collect.SystemdUnit{{Name: "nginx.service"}}
	m.logs.cancel = func() { cancelled++ }
	m.logs.buffer.Append("INFO ready")
	m.logs.buffer.Append("ERROR failed")

	// When: pause, filter, source-cycle, reconnect and escape are requested.
	m, _ = updateModel(t, m, key(" "))
	if !m.logs.paused || len(m.logs.buffer.Visible()) != 2 {
		t.Fatalf("pause state = %#v", m.logs)
	}
	m, _ = updateModel(t, m, key("/"))
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("error")})
	m, _ = updateModel(t, m, key("enter"))
	if got := m.logs.buffer.Visible(); len(got) != 1 || got[0] != "ERROR failed" {
		t.Fatalf("filtered = %#v", got)
	}
	oldGeneration := m.logs.generation
	m, _ = updateModel(t, m, key("s"))
	if m.logs.source != 1 || m.logs.generation <= oldGeneration {
		t.Fatalf("source=%d generation=%d", m.logs.source, m.logs.generation)
	}
	m.logs.cancel = func() { cancelled++ }
	beforeReconnect := m.logs.generation
	m, _ = updateModel(t, m, key("r"))
	if m.logs.generation <= beforeReconnect {
		t.Fatal("reconnect did not advance generation")
	}
	m.logs.cancel = func() { cancelled++ }
	m, _ = updateModel(t, m, key("esc"))

	// Then: leaving cancels the stream and returns to Dashboard.
	if m.screen != screenDashboard || cancelled < 3 {
		t.Fatalf("screen=%v cancelled=%d", m.screen, cancelled)
	}
}

func TestLogsBufferAndViewportStayBoundedWhilePausedAndResized(t *testing.T) {
	// Given: more than ten thousand lines in a paused log screen.
	m := Model{screen: screenLogs, logs: newLogsScreen()}
	for i := 0; i < 10_005; i++ {
		m.logs.buffer.Append("line")
	}
	m, _ = updateModel(t, m, key(" "))

	// When: the terminal is resized to its minimum supported height.
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 60, Height: 16})
	view := m.View()

	// Then: storage remains bounded and the viewport has a valid height.
	if len(m.logs.buffer.Visible()) != 10_000 {
		t.Fatalf("visible lines = %d", len(m.logs.buffer.Visible()))
	}
	if m.logs.viewport.Height <= 0 || view == "" {
		t.Fatalf("viewport height=%d view=%q", m.logs.viewport.Height, view)
	}
}

func logsScreenModel(t *testing.T, lines ...string) Model {
	t.Helper()
	m := Model{screen: screenLogs, snapshot: snapshotWithServers("web"), logs: newLogsScreen()}
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
	for _, line := range lines {
		m.logs.buffer.Append(line)
	}
	m.logs.refresh()
	return m
}

func TestLogsSourcesComeFromUnitsAndContainers(t *testing.T) {
	// Given: a server whose systemd units and docker containers are already loaded.
	streamer := &fakeLogStreamer{}
	m := Model{screen: screenDashboard, snapshot: snapshotWithServers("web"), logSource: streamer, logs: newLogsScreen()}
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
	m.dashboard.server = "web"
	m.dashboard.units.items = []collect.SystemdUnit{{Name: "postgres.service"}}
	m.dashboard.containers.items = []collect.Container{{ID: "abc", Name: "api-worker"}}

	// When: the full screen logs are opened.
	m, cmd := updateModel(t, m, key("l"))
	if cmd != nil {
		cmd()
	}

	// Then: the axis offers the system journal first, then units and containers.
	want := []collect.LogSource{
		{Kind: collect.LogSystem},
		{Kind: collect.LogJournal, Name: "postgres.service"},
		{Kind: collect.LogContainer, Name: "api-worker"},
	}
	if !slices.Equal(m.logs.sources, want) {
		t.Fatalf("sources = %#v", m.logs.sources)
	}
	axis := m.logsSourceAxis(100)
	for _, fragment := range []string{"systemd", "postgres", "docker/api-worker", "1/3 · ← →"} {
		if !strings.Contains(axis, fragment) {
			t.Fatalf("axis %q misses %q", axis, fragment)
		}
	}
}

func TestLogsSourceSwitchRestartsStreamAndDropsLines(t *testing.T) {
	// Given: a logs screen with a container source discovered on the server.
	streamer := &fakeLogStreamer{streams: []collect.LogStream{
		{Lines: make(chan string, 1), Errors: make(chan error, 1), Close: func() error { return nil }},
		{Lines: make(chan string, 1), Errors: make(chan error, 1), Close: func() error { return nil }},
	}}
	m := Model{screen: screenLogs, snapshot: snapshotWithServers("web"), logSource: streamer, logs: newLogsScreen()}
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
	m.dashboard.server = "web"
	m.dashboard.containers.items = []collect.Container{{ID: "abc", Name: "api-worker"}}
	m.logs.buffer.Append("system line")

	// When: the source is switched forward with "s" and the command runs.
	m, cmd := updateModel(t, m, key("s"))
	if cmd == nil {
		t.Fatal("source switch produced no command")
	}
	cmd()

	// Then: the container source is streamed and the previous lines are gone.
	if m.logs.source != 1 || len(m.logs.buffer.Visible()) != 0 {
		t.Fatalf("source=%d visible=%#v", m.logs.source, m.logs.buffer.Visible())
	}
	if len(streamer.requests) != 1 || streamer.requests[0].Source.Name != "api-worker" {
		t.Fatalf("requests = %#v", streamer.requests)
	}

	// And: switching back wraps to the system journal.
	m, cmd = updateModel(t, m, key("left"))
	if cmd != nil {
		cmd()
	}
	if m.logs.source != 0 {
		t.Fatalf("source after wrap = %d", m.logs.source)
	}
}

func TestLogsSourcesFollowServerChangeAndSurviveEmptyData(t *testing.T) {
	// Given: a logs screen on a server without collected units or containers.
	streamer := &fakeLogStreamer{}
	m := Model{screen: screenLogs, snapshot: snapshotWithServers("web", "db"), logSource: streamer, logs: newLogsScreen()}
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
	m.startLogsStream()

	// Then: a single system source stays selectable and switching does not panic.
	if len(m.logs.sources) != 1 {
		t.Fatalf("sources = %#v", m.logs.sources)
	}
	m, _ = updateModel(t, m, key("s"))
	if m.logs.source != 0 {
		t.Fatalf("source without alternatives = %d", m.logs.source)
	}

	// When: another server is selected and its units are known.
	m.selected = 1
	m.dashboard.server = "db"
	m.dashboard.units.items = []collect.SystemdUnit{{Name: "redis.service"}}
	m.startLogsStream()

	// Then: the list is rebuilt for the new server.
	if len(m.logs.sources) != 2 || m.logs.sources[1].Name != "redis.service" {
		t.Fatalf("sources after server change = %#v", m.logs.sources)
	}
}

func TestLogsSourcesIgnoreUnitsAndContainersOfAnotherServer(t *testing.T) {
	// Given: юниты и контейнеры, загруженные экраном сервера «web».
	streamer := &fakeLogStreamer{}
	m := Model{screen: screenLogs, snapshot: snapshotWithServers("web", "db"), logSource: streamer, logs: newLogsScreen()}
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
	m.dashboard.server = "web"
	m.dashboard.units.items = []collect.SystemdUnit{{Name: "postgres.service"}}
	m.dashboard.containers.items = []collect.Container{{ID: "abc", Name: "api-worker"}}

	// When: логи открывают для другого сервера.
	m.selected = 1
	m.startLogsStream()

	// Then: в оси остаётся только системный журнал — чужой юнит дал бы пустой
	// вывод, а чужой контейнер — ошибку.
	if !slices.Equal(m.logs.sources, []collect.LogSource{{Kind: collect.LogSystem}}) {
		t.Fatalf("sources of another server = %#v", m.logs.sources)
	}

	// And: у своего сервера источники на месте.
	m.selected = 0
	m.startLogsStream()
	if len(m.logs.sources) != 3 {
		t.Fatalf("sources of the own server = %#v", m.logs.sources)
	}
}

func TestLogsWarnOnlyTogglesInsteadOfCycling(t *testing.T) {
	// Given: a logs screen with lines of every level.
	m := logsScreenModel(t, "info ready", "WARN disk almost full", "ERROR write failed")

	// When: "w" is pressed.
	m, _ = updateModel(t, m, key("w"))

	// Then: warn and above stay visible and the level does not walk to "info".
	if m.logs.level != logLevelWarn || len(m.logs.visibleLines()) != 2 {
		t.Fatalf("level=%v visible=%#v", m.logs.level, m.logs.visibleLines())
	}

	// When: "w" is pressed again.
	m, _ = updateModel(t, m, key("w"))

	// Then: the filter is off again, and the cycle lives on "W".
	if m.logs.level != logLevelAll {
		t.Fatalf("level after second toggle = %v", m.logs.level)
	}
	m, _ = updateModel(t, m, key("W"))
	if m.logs.level != logLevelInfo {
		t.Fatalf("level after cycle = %v", m.logs.level)
	}
}

func TestLogsCursorSelectionAndContextWindow(t *testing.T) {
	// Given: a filtered logs screen where neighbours are hidden by the filter.
	m := logsScreenModel(t, "a1 noise", "b keep", "a2 noise", "a3 noise", "keep tail")
	m, _ = updateModel(t, m, key("/"))
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("keep")})
	m, _ = updateModel(t, m, key("enter"))

	// When: the cursor is moved up twice.
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if m.logs.cursor != 1 {
		t.Fatalf("cursor after first move = %d", m.logs.cursor)
	}
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if m.logs.cursor != 0 {
		t.Fatalf("cursor after second move = %d", m.logs.cursor)
	}

	// Then: the selected line is marked in the body.
	if view := m.View(); !strings.Contains(view, "▍") {
		t.Fatalf("selection marker missing:\n%s", view)
	}

	// When: the context mode is entered on the last match.
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m, _ = updateModel(t, m, key("c"))

	// Then: neighbours hidden by the filter are shown around the selected line.
	if len(m.logs.contextLines) != 5 || !slices.Contains(m.logs.contextLines, "a2 noise") {
		t.Fatalf("context = %#v", m.logs.contextLines)
	}
	if line, ok := m.logs.selectedLine(); !ok || line != "keep tail" {
		t.Fatalf("selected line in context = %q ok=%v", line, ok)
	}

	// And: escape leaves the context mode without leaving the screen.
	m, _ = updateModel(t, m, key("esc"))
	if m.logs.contextLines != nil || m.screen != screenLogs {
		t.Fatalf("context=%#v screen=%v", m.logs.contextLines, m.screen)
	}
}

func TestLogsMatchJumpAndCopyNotice(t *testing.T) {
	// Given: a logs screen with the last line selected.
	m := logsScreenModel(t, "one", "two", "three")
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyUp})

	// When: previous and next matches are requested.
	m, _ = updateModel(t, m, key("N"))
	if m.logs.cursor != 1 {
		t.Fatalf("cursor after N = %d", m.logs.cursor)
	}
	m, _ = updateModel(t, m, key("n"))
	if m.logs.cursor != 2 {
		t.Fatalf("cursor after n = %d", m.logs.cursor)
	}

	// And: copying reports back in the status line.
	m, cmd := updateModel(t, m, key("y"))
	if cmd == nil || m.logs.notice == "" {
		t.Fatalf("copy cmd=%v notice=%q", cmd, m.logs.notice)
	}
	if !strings.Contains(m.View(), m.logs.notice) {
		t.Fatalf("notice missing from the header:\n%s", m.View())
	}
	if payload := osc52Copy("three"); payload != "\x1b]52;c;dGhyZWU=\a" {
		t.Fatalf("osc52 payload = %q", payload)
	}
}

// logsScreenWithLines — экран логов, где строк заведомо больше высоты окна.
func logsScreenWithLines(t *testing.T, count int) Model {
	t.Helper()
	lines := make([]string, 0, count)
	for i := range count {
		lines = append(lines, fmt.Sprintf("line %02d", i))
	}
	return logsScreenModel(t, lines...)
}

func TestLogsHomeAndEndMoveToTheEdges(t *testing.T) {
	// Given: экран логов с сорока строками в окне на шестнадцать.
	m := logsScreenWithLines(t, 40)

	// When: нажат home.
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyHome})

	// Then: окно в начале буфера, и пришедшая строка его оттуда не уводит.
	if m.logs.viewport.YOffset != 0 || !strings.Contains(m.View(), "line 00") {
		t.Fatalf("home не увёл окно в начало: YOffset = %d\n%s", m.logs.viewport.YOffset, m.View())
	}
	m.logs.buffer.Append("line 40")
	m.logs.refresh()
	if m.logs.viewport.YOffset != 0 {
		t.Fatalf("новая строка увела окно от начала: YOffset = %d", m.logs.viewport.YOffset)
	}

	// When: нажат end.
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnd})

	// Then: выделение снято, окно у хвоста.
	if m.logs.cursor != -1 || m.logs.viewport.YOffset == 0 {
		t.Fatalf("cursor=%d YOffset=%d", m.logs.cursor, m.logs.viewport.YOffset)
	}
	if !strings.Contains(m.View(), "line 40") {
		t.Fatalf("после end хвост не виден:\n%s", m.View())
	}
}

func TestLogsPageUpSurvivesIncomingLines(t *testing.T) {
	// Given: экран логов, следящий за хвостом потока.
	m := logsScreenWithLines(t, 40)

	// When: страница пролистана вверх и приходит новая строка.
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyPgUp})
	offset, cursor := m.logs.viewport.YOffset, m.logs.cursor
	m.logs.buffer.Append("fresh line")
	m.logs.refresh()

	// Then: окно осталось там, куда его пролистали, — выделение его удерживает.
	if cursor < 0 || offset == 0 {
		t.Fatalf("pgup не сдвинул окно: cursor=%d offset=%d", cursor, offset)
	}
	if m.logs.viewport.YOffset != offset || m.logs.cursor != cursor {
		t.Fatalf("страницу отмотало назад: YOffset=%d (было %d) cursor=%d", m.logs.viewport.YOffset, offset, m.logs.cursor)
	}
}

func TestLogsDensitySpansMidnight(t *testing.T) {
	// Given: строки, пришедшие до и после полуночи.
	density := newLogDensity([]string{
		"23:58:00 info вечерняя запись",
		"23:59:30 warn почти полночь",
		"00:01:00 info уже завтра",
	}, 8)

	// Then: диапазон — три минуты, а не почти сутки назад.
	if density.span != "-3м — сейчас" {
		t.Fatalf("span = %q", density.span)
	}
	if !strings.Contains(density.spike, "23:59") {
		t.Fatalf("spike = %q", density.spike)
	}
}

func TestLogsDensitySurvivesEmptyBufferAndLinesWithoutTime(t *testing.T) {
	// Given: no lines at all.
	empty := newLogDensity(nil, 10)
	if empty.counts != nil || empty.spike != "" || empty.span != "" {
		t.Fatalf("empty density = %#v", empty)
	}

	// And: lines without any timestamp.
	untimed := newLogDensity([]string{"boot ok", "still ok"}, 10)
	if untimed.counts != nil || untimed.spike != "" {
		t.Fatalf("untimed density = %#v", untimed)
	}

	// When: timestamped lines with a warn burst are bucketed.
	density := newLogDensity([]string{
		"19:41:02 info postgres checkpoint starting",
		"19:48:31 warn postgres checkpoints are occurring too frequently",
		"19:48:35 warn postgres checkpoints are occurring too frequently",
		"19:53:58 info postgres checkpoint starting",
	}, 8)

	// Then: the band is filled, the burst is labelled and the range is shown.
	if len(density.counts) != 8 || density.counts[0] == nil {
		t.Fatalf("counts = %#v", density.counts)
	}
	if !strings.Contains(density.spike, "всплеск warn") || !strings.Contains(density.spike, "19:4") {
		t.Fatalf("spike = %q", density.spike)
	}
	if density.span != "-12м — сейчас" {
		t.Fatalf("span = %q", density.span)
	}

	// And: the rendered band never panics on a screen without lines.
	m := logsScreenModel(t)
	if line := m.logsDensityLine(80); !strings.Contains(line, "плотность") {
		t.Fatalf("density line = %q", line)
	}
}

func TestLogsFooterKeepsSubsetAndCloseHintWhenNarrow(t *testing.T) {
	// Given: the narrowest supported terminal and a wide one.
	narrow := strings.Join(logsFooter(minimumWidth), "\n")
	wide := strings.Join(logsFooter(160), "\n")

	// Then: both advertise the same keys, the narrow one only fewer of them.
	for _, footer := range []string{narrow, wide} {
		for _, want := range []string{"/ фильтр", "esc закрыть"} {
			if !strings.Contains(footer, want) {
				t.Fatalf("footer %q misses %q", footer, want)
			}
		}
	}
	if !strings.Contains(wide, "y копировать") {
		t.Fatalf("wide footer misses the copy hint: %q", wide)
	}
	for _, row := range logsFooter(minimumWidth) {
		if lipgloss.Width(row) > minimumWidth {
			t.Fatalf("footer row %q is wider than %d", row, minimumWidth)
		}
	}
}

func TestLogsTimeColumnTogglesWithT(t *testing.T) {
	// Given: a logs screen showing a journal line with a timestamp prefix.
	m := logsScreenModel(t, "Aug 01 19:41:02 vm postgres[812]: checkpoint starting")

	// When: the time column is switched off.
	m, _ = updateModel(t, m, key("t"))

	// Then: the prefix up to the clock is dropped, the message stays.
	if got := m.logs.displayLine("Aug 01 19:41:02 vm postgres[812]: checkpoint starting"); got != "vm postgres[812]: checkpoint starting" {
		t.Fatalf("line without time = %q", got)
	}
	// And: lines without a timestamp are left as they are.
	if got := m.logs.displayLine("no clock here"); got != "no clock here" {
		t.Fatalf("untimed line = %q", got)
	}
}

func TestLogsNeverPassRemoteEscapeSequencesToTheTerminal(t *testing.T) {
	// Given: экран логов, куда чужой процесс написал OSC 52 (перехват буфера
	// обмена), OSC 0 (подмена заголовка окна) и CSI (порча раскладки).
	m := logsScreenModel(t,
		"nginx: \x1b]52;c;ZXZpbA==\x07GET /health",
		"\x1b]0;pwned\x07docker: started",
		"\x1b[31mERROR\x1b[2Kdisk full",
	)

	// When: кадр отрисован.
	view := m.View()

	// Then: ни одной пришедшей с сервера последовательности в кадре нет,
	// а текст строк на месте.
	for _, forbidden := range []string{"\x1b]52", "\x1b]0;", "\x1b[31m", "\x1b[2K"} {
		if strings.Contains(view, forbidden) {
			t.Fatalf("кадр содержит %q:\n%q", forbidden, view)
		}
	}
	for _, want := range []string{"GET /health", "docker: started", "ERRORdisk full"} {
		if !strings.Contains(view, want) {
			t.Fatalf("кадр потерял текст %q:\n%s", want, view)
		}
	}

	// And: в теле экрана не осталось управляющих байтов вовсе — двигать курсор
	// и чистить строки чужому логу нечем, раскладка кадра остаётся за нами.
	for _, line := range m.logs.visibleLines() {
		if index := strings.IndexFunc(line, func(r rune) bool { return r < 0x20 || r == 0x7f }); index >= 0 {
			t.Fatalf("управляющий байт в строке %q на позиции %d", line, index)
		}
	}
}

func TestLogsHighlightIsAppliedOverSanitizedText(t *testing.T) {
	// Given: строка с CSI-цветом, уложенная в буфер тем же путём, что и живой поток.
	buffer := collect.NewLogBuffer(10)
	buffer.Append("\x1b[31mERROR\x1b[0m disk full")
	line := buffer.Visible()[0]

	// When: поверх неё накладывается подсветка совпадений фильтра.
	got := highlightMatches(line, "disk")

	// Then: подсвечен ровно текст, а не сдвинутый escape-последовательностями
	// кусок строки — то есть санитизация прошла раньше подсветки.
	if want := "ERROR " + warnStyle.Render("disk") + " full"; got != want {
		t.Fatalf("highlight = %q, want %q", got, want)
	}
	if lipgloss.Width(line) != len("ERROR disk full") {
		t.Fatalf("ширина санитизированной строки = %d", lipgloss.Width(line))
	}
}

func TestLogsClipboardPayloadIsCapped(t *testing.T) {
	// Given: строка лога длиннее предела OSC 52 (сканер потока пропускает до 1 МиБ).
	long := strings.Repeat("a", clipboardLimit*3)
	m := logsScreenModel(t, long)
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyUp})

	// When: строку копируют по «y».
	m, cmd := updateModel(t, m, key("y"))

	// Then: в терминал уходит ограниченная последовательность, а обрезка названа
	// в подтверждающем сообщении.
	if cmd == nil {
		t.Fatal("копирование не вернуло команду")
	}
	if payload := osc52Copy(long); len(payload) > 2*clipboardLimit {
		t.Fatalf("длина последовательности = %d", len(payload))
	}
	if !strings.Contains(m.logs.notice, "обрезана") {
		t.Fatalf("notice = %q", m.logs.notice)
	}

	// And: короткая строка копируется целиком и без пометки.
	short := logsScreenModel(t, "short line")
	short, _ = updateModel(t, short, tea.KeyMsg{Type: tea.KeyUp})
	short, _ = updateModel(t, short, key("y"))
	if short.logs.notice != "строка скопирована" {
		t.Fatalf("notice короткой строки = %q", short.logs.notice)
	}
}
