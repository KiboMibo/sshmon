package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kibomibo/sshmon/internal/collect"
)

func TestFleetSidebarProcessRequestsAreDebounced(t *testing.T) {
	// Given: экран флота с видимым сайдбаром «ТОП ПО ПАМЯТИ».
	m := Model{
		screen:   screenFleet,
		snapshot: snapshotWithServers("a", "b", "c", "d"),
		fleet:    newFleetModel(),
		layout:   newLayout(120, 30),
	}
	if !m.fleetSidebarVisible() {
		t.Fatal("сайдбар не виден — тест проверял бы не то")
	}

	// When: стрелка «вниз» удержана и дала три события подряд.
	ticks := make([]debounceMsg, 0, 3)
	for range 3 {
		next, cmd := updateModel(t, m, key("j"))
		m = next
		if cmd == nil {
			t.Fatal("движение курсора не запланировало запрос")
		}
		ticks = append(ticks, debounceMsg{kind: debounceTopProcesses, generation: m.processes.generation})
	}

	// Then: тики от промежуточных нажатий отброшены по поколению, и `ps` уходит
	// один раз — за последним хостом, на котором курсор остановился.
	started := 0
	for _, tick := range ticks {
		next, cmd := updateModel(t, m, tick)
		m = next
		if cmd != nil {
			started++
		}
	}
	if started != 1 {
		t.Fatalf("запросов ps = %d, ожидался один", started)
	}
	if m.selectedName() != "d" {
		t.Fatalf("курсор остановился на %q", m.selectedName())
	}
}

func TestFleetLogboxRapidMovementOpensOneStream(t *testing.T) {
	// Given: открытый ящик логов под выбранной строкой списка из четырёх хостов.
	streamer := &fakeLogStreamer{streams: []collect.LogStream{
		{Lines: make(chan string, 1), Errors: make(chan error, 1), Close: func() error { return nil }},
		{Lines: make(chan string, 1), Errors: make(chan error, 1), Close: func() error { return nil }},
	}}
	m := Model{
		screen:    screenFleet,
		snapshot:  snapshotWithServers("a", "b", "c", "d"),
		logSource: streamer,
		logs:      newLogsScreen(),
		layout:    newLayout(120, 30),
		fleet:     newFleetModel(),
	}
	opened, cmd := updateModel(t, m, key("l"))
	if cmd != nil {
		cmd()
	}

	// When: стрелка «вниз» удержана и дала три события подряд.
	ticks := make([]debounceMsg, 0, 3)
	for range 3 {
		next, _ := updateModel(t, opened, key("j"))
		opened = next
		ticks = append(ticks, debounceMsg{kind: debounceLogs, generation: opened.logs.generation})
	}
	if len(streamer.requests) != 1 {
		t.Fatalf("поток открыт до конца паузы: %#v", streamer.requests)
	}

	// Then: после паузы открывается ровно один новый поток — на последнем хосте.
	for _, tick := range ticks {
		var cmd tea.Cmd
		opened, cmd = updateModel(t, opened, tick)
		if cmd != nil {
			cmd()
		}
	}
	if len(streamer.requests) != 2 || streamer.requests[1].Server != "d" {
		t.Fatalf("requests = %#v", streamer.requests)
	}
}
