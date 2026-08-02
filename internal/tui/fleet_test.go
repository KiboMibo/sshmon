package tui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kibomibo/sshmon/internal/collect"
	"github.com/kibomibo/sshmon/internal/config"
)

// stripANSI убирает из кадра управляющие последовательности: проверки на
// «строка кончается вплотную к рамке» иначе спотыкались бы о сброс стиля,
// который стоит между последним числом и границей панели.
func stripANSI(value string) string {
	runes := []rune(value)
	var out strings.Builder
	for index := 0; index < len(runes); {
		if runes[index] == 0x1b {
			index = escapeEnd(runes, index)
			continue
		}
		out.WriteRune(runes[index])
		index++
	}
	return out.String()
}

func TestFleetRenderAdaptsPreviewAndHasNoTabs(t *testing.T) {
	// Given an online selected server with metrics and a problem.
	now := time.Now().Add(-7 * time.Second)
	snapshot := collect.Snapshot{Time: time.Now(), Servers: []collect.Metrics{{Name: "web", Group: "prod", Online: true, Time: now, Hostname: "web-01", CPUPct: 42, MemPct: 51, Load1: 1.2}}, Issues: []collect.Issue{{Server: "web", Severity: "warn", Msg: "CPU"}}}
	m := Model{screen: screenFleet, snapshot: snapshot, config: &config.Config{Servers: []config.Server{{Name: "web", Host: "10.0.0.1", Group: "prod"}}}}
	// When rendered at wide and narrow sizes.
	m.layout = newLayout(120, 30)
	wide := m.View()
	m.layout = newLayout(80, 24)
	narrow := m.View()
	// Then wide includes preview, narrow hides it, and old tabs are absent.
	if !strings.Contains(wide, "web-01") || !strings.Contains(wide, "42%") || strings.Contains(narrow, "web-01") {
		t.Fatalf("wide=%q narrow=%q", wide, narrow)
	}
	if strings.Contains(wide, "1:Overview") || !strings.Contains(wide, "●") {
		t.Fatalf("fleet view = %q", wide)
	}
}

func TestFleetKeysClampAndToggleFilters(t *testing.T) {
	// Given a Fleet containing three grouped servers.
	m := Model{screen: screenFleet, snapshot: collect.Snapshot{Servers: []collect.Metrics{{Name: "a", Group: "prod"}, {Name: "b", Group: "data"}, {Name: "c", Group: "prod"}}}}
	// When paging, cycling groups, toggling problems and preview.
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyPgDown})
	m, _ = updateModel(t, m, key("g"))
	m, _ = updateModel(t, m, key("!"))
	m, _ = updateModel(t, m, key("v"))
	// Then navigation follows grouped display order and Fleet state changes deterministically.
	if m.selected != 0 || m.fleet.filter.Group != "prod" || !m.fleet.filter.ProblemsOnly || m.fleet.preview {
		t.Fatalf("selected=%d filter=%+v preview=%v", m.selected, m.fleet.filter, m.fleet.preview)
	}
}

func TestFleetGroupsServersUnderExplicitHeadings(t *testing.T) {
	// Given servers from two groups interleaved in config order.
	m := Model{screen: screenFleet, snapshot: collect.Snapshot{Servers: []collect.Metrics{{Name: "alpha", Group: "prod"}, {Name: "bravo", Group: "data"}, {Name: "charlie", Group: "prod"}}}}
	m.layout = newLayout(80, 24)
	// When the Fleet screen is rendered.
	view := m.View()
	// Then each group heading appears exactly once and rows follow grouped order.
	if strings.Count(view, "prod") != 1 || strings.Count(view, "data") != 1 {
		t.Fatalf("group headings duplicated or missing: %q", view)
	}
	prod, alpha, charlie, data, bravo := strings.Index(view, "prod"), strings.Index(view, "alpha"), strings.Index(view, "charlie"), strings.Index(view, "data"), strings.Index(view, "bravo")
	if !(prod < alpha && alpha < charlie && charlie < data && data < bravo) {
		t.Fatalf("grouped order broken: prod=%d alpha=%d charlie=%d data=%d bravo=%d", prod, alpha, charlie, data, bravo)
	}
}

func TestFleetRowShowsServerUptimeInsteadOfDataAge(t *testing.T) {
	// Given an online server reporting a multi-day uptime.
	m := Model{screen: screenFleet, snapshot: collect.Snapshot{Time: time.Now(), Servers: []collect.Metrics{{Name: "web", Group: "prod", Online: true, Time: time.Now().Add(-7 * time.Second), Uptime: 50*time.Hour + 30*time.Minute}}}}
	m.layout = newLayout(80, 24)
	// When the row is expanded, where the uptime column belongs.
	m, _ = updateModel(t, m, key("right"))
	view := m.View()
	// Then the table shows the server uptime in the layout format, not the sample age.
	if !strings.Contains(view, "UPTIME") || !strings.Contains(view, "2д") || strings.Contains(view, "ВОЗРАСТ") {
		t.Fatalf("fleet view = %q", view)
	}
}

func TestFleetExpandedKeepsSidebarBesideTheCard(t *testing.T) {
	// Given a wide fleet with the sidebar enabled.
	snapshot := collect.Snapshot{Time: time.Now(), Servers: []collect.Metrics{{
		Name: "kava", Group: "main", Online: true, Time: time.Now(), Hostname: "10.2.4.18",
		NumCPU: 8, CPUPct: 16, MemPct: 61, MemTotalKB: 16 * 1024 * 1024, MemAvailKB: 6 * 1024 * 1024,
	}}}
	m := Model{screen: screenFleet, snapshot: snapshot, layout: newLayout(140, 40), fleet: newFleetModel()}
	// When the sidebar is visible, it carries the host details.
	if view := m.View(); !strings.Contains(view, "ЧТО НЕ ТАК") || !strings.Contains(view, "ДЕЙСТВИЯ") {
		t.Fatalf("sidebar missing before expansion:\n%s", view)
	}
	// When the row is expanded with the right arrow.
	m, _ = updateModel(t, m, key("right"))
	view := m.View()
	// Then the sidebar stays: контекст «что не так» нужен именно в этот момент
	// (осознанное расхождение с макетом 3b).
	for _, want := range []string{"ЧТО НЕ ТАК", "ТОП ПО ПАМЯТИ", "ДЕЙСТВИЯ"} {
		if !strings.Contains(view, want) {
			t.Fatalf("sidebar section %q lost on expansion:\n%s", want, view)
		}
	}
	if !strings.Contains(view, "ядер") || !strings.Contains(view, "10.2.4.18") {
		t.Fatalf("expanded card missing:\n%s", view)
	}
	// And each frame line still fits the terminal: карточка ужимается, а не
	// вылезает за правую рамку сайдбара.
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > 140 {
			t.Fatalf("строка кадра в %d ячеек при ширине 140: %q", lipgloss.Width(line), line)
		}
	}
}

func TestFleetStateColumnCarriesTextAndSelectionMarker(t *testing.T) {
	// Given an offline host, a host with a problem and a healthy selected host.
	snapshot := collect.Snapshot{Time: time.Now(), Servers: []collect.Metrics{
		{Name: "web", Group: "prod", Online: true, Time: time.Now()},
		{Name: "db", Group: "prod", Online: true, Time: time.Now()},
		{Name: "arb", Group: "prod", Time: time.Now()},
	}, Issues: []collect.Issue{{Server: "db", Severity: "warn", Msg: "память 98%"}}}
	m := Model{screen: screenFleet, snapshot: snapshot, layout: newLayout(120, 30), fleet: newFleetModel()}
	// When the list is rendered.
	view := m.View()
	// Then the state column reads without colour and the cursor row is marked.
	for _, want := range []string{"ХОСТ", "СОСТ", "● норма", "⚠ память 98%", "× нет связи", fleetMarker} {
		if !strings.Contains(view, want) {
			t.Fatalf("fleet list misses %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "ИМЯ") {
		t.Fatalf("first column is still named ИМЯ:\n%s", view)
	}
}

func TestFleetColumnsAppearOnlyWithDetailsAndDegrade(t *testing.T) {
	// Given the list mode and the details mode at the same width.
	plain := fleetColumnLayout(120, false)
	detailed := fleetColumnLayout(120, true)
	// Then uptime and docker belong to the details mode only.
	if plain.uptime || plain.docker {
		t.Fatalf("list mode shows extra columns: %+v", plain)
	}
	if !detailed.uptime || !detailed.docker {
		t.Fatalf("details mode misses columns: %+v", detailed)
	}
	if !strings.Contains(detailed.header(), "UPTIME") || strings.Contains(plain.header(), "DOCKER") {
		t.Fatalf("headers = %q / %q", detailed.header(), plain.header())
	}
	// And on a narrow terminal the extra columns leave first, docker before uptime.
	narrow := fleetColumnLayout(70, true)
	if narrow.docker || !narrow.uptime {
		t.Fatalf("narrow details layout = %+v", narrow)
	}
	// На 60 колонках уходят обе: DISK принадлежит базовому набору и остаётся.
	tight := fleetColumnLayout(60, true)
	if tight.docker || tight.uptime {
		t.Fatalf("tight details layout = %+v", tight)
	}
	if !strings.Contains(tight.header(), "DISK") {
		t.Fatalf("колонка DISK ушла вместе с деталями: %q", tight.header())
	}
	if narrow.name < fleetNameMin {
		t.Fatalf("host column shrank below the minimum: %+v", narrow)
	}
}

// columnEnd — на какой ячейке строки заканчивается value.
func columnEnd(line, value string) int {
	index := strings.Index(line, value)
	if index < 0 {
		return -1
	}
	return lipgloss.Width(line[:index+len(value)])
}

func TestFleetTableFillsPanelWidth(t *testing.T) {
	// Given: список хостов на терминалах разной ширины, в обоих режимах колонок.
	for _, width := range []int{60, 80, 100, 160, 200} {
		for _, detailed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/%v", width, detailed), func(t *testing.T) {
				cols := fleetColumnLayout(width, detailed)
				if cols.fixed() > width {
					// Ширины не хватает даже на минимум — зазоры базовые, поведение прежнее.
					if cols.lead != fleetGapWidth || cols.inner != fleetGapWidth {
						t.Fatalf("зазоры разъехались на тесной раскладке: %+v", cols)
					}
					return
				}

				// When: рисуются заголовок и строка хоста.
				header := cols.header()
				row := strings.Repeat(" ", len([]rune(fleetMarker))) +
					// Значения ячеек различны нарочно: columnEnd ищет первое
					// вхождение, и «96%» из колонки СОСТ перебило бы MEM.
					cols.row("vm-prod-emarb", "⚠ память 96%", "2%", "94%", "88%", "0.79", "1718д", "●7 ○2 ⚠1")

				// Then: обе строки ровно по ширине панели и колонки совпадают.
				if got := lipgloss.Width(header); got != width {
					t.Fatalf("заголовок в %d ячеек при ширине панели %d: %q", got, width, header)
				}
				if got := lipgloss.Width(row); got != width {
					t.Fatalf("строка в %d ячеек при ширине панели %d: %q", got, width, row)
				}
				// Смещение считаем в ячейках, а не в байтах: в колонках кириллица.
				for _, pair := range [][2]string{{"CPU", "2%"}, {"MEM", "94%"}, {"DISK", "88%"}, {"LOAD", "0.79"}} {
					if columnEnd(header, pair[0]) != columnEnd(row, pair[1]) {
						t.Fatalf("колонка %s не под своим заголовком:\n%s\n%s", pair[0], header, row)
					}
				}
			})
		}
	}
}

// TestFleetNumericGapsGrowWithPanelWidth — Дано: одна и та же таблица на
// панелях разной ширины; Тогда: интервалы между числовыми колонками растут
// вместе с панелью, а не только отступ перед блоком чисел. Без этого числа
// снова слиплись бы у правой рамки в «4%  51%  0.00».
func TestFleetNumericGapsGrowWithPanelWidth(t *testing.T) {
	narrow, wide := fleetColumnLayout(80, false), fleetColumnLayout(200, false)
	if wide.inner <= narrow.inner {
		t.Fatalf("зазор между числами не вырос: %d при 80 против %d при 200", narrow.inner, wide.inner)
	}
	if narrow.inner < fleetGapWidth {
		t.Fatalf("зазор уже базового: %+v", narrow)
	}
	// И: разбег между зазорами не больше остатка от деления — блок чисел
	// разложен равномерно, а не «дырка перед CPU и слипшийся хвост».
	if wide.lead-wide.inner >= len(wide.numericWidths()) {
		t.Fatalf("ширина ушла в один отступ: lead=%d inner=%d", wide.lead, wide.inner)
	}
}

// TestFleetDiskColumnPrefersRootPartition — Дано: хосты с разным набором
// разделов; Тогда: в колонке DISK стоит «/», если он собран, иначе самый
// заполненный раздел, а у offline-хоста — прочерк.
func TestFleetDiskColumnPrefersRootPartition(t *testing.T) {
	// Ширина 80: список в одну колонку без рамки и сайдбара, поэтому последние
	// поля строки — это ровно ячейки DISK и LOAD.
	for _, tc := range []struct {
		name   string
		online bool
		disks  []collect.DiskUsage
		want   string
	}{
		{"корень среди прочих", true, []collect.DiskUsage{{Mount: "/var", UsedPct: 97}, {Mount: "/", UsedPct: 41}, {Mount: "/boot", UsedPct: 12}}, "41%"},
		{"корня нет", true, []collect.DiskUsage{{Mount: "/data", UsedPct: 63}, {Mount: "/srv", UsedPct: 88}}, "88%"},
		{"дисков нет", true, nil, "—"},
		{"нет связи", false, []collect.DiskUsage{{Mount: "/", UsedPct: 41}}, "—"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := collect.Snapshot{Time: time.Now(), Servers: []collect.Metrics{
				{Name: "kava", Group: "main", Online: tc.online, Time: time.Now(), Disks: tc.disks},
			}}
			m := Model{screen: screenFleet, snapshot: snapshot, fleet: newFleetModel(), layout: newLayout(80, 24)}
			view := stripANSI(m.View())
			if !strings.Contains(view, "DISK") {
				t.Fatalf("колонки DISK нет в списке:\n%s", view)
			}
			cols := strings.Fields(fleetRowOf(t, view, "kava"))
			if got := cols[len(cols)-2]; got != tc.want {
				t.Fatalf("колонка DISK = %q, want %q:\n%s", got, tc.want, view)
			}
		})
	}
}

// TestFleetDockerColumnTellsThreeStates — Дано: три хоста в разном состоянии
// docker'а; Тогда: колонка различает «не ответил», «контейнеров нет» и живые
// счётчики — тем же видом «●7 ○2 ⚠1», что заголовок плитки экрана сервера.
func TestFleetDockerColumnTellsThreeStates(t *testing.T) {
	for _, tc := range []struct {
		name   string
		counts collect.DockerCounts
		want   string
	}{
		{"docker не ответил", collect.DockerCounts{}, "—"},
		{"контейнеров нет", collect.DockerCounts{Known: true}, "●0"},
		{"контейнеры есть", collect.DockerCounts{Running: 7, Stopped: 2, Broken: 1, Known: true}, "●7 ○2 ⚠1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := dockerCell(tc.counts); got != tc.want {
				t.Fatalf("dockerCell(%+v) = %q, want %q", tc.counts, got, tc.want)
			}
		})
	}
	// И: счётчики доезжают до строки списка из снапшота, а не только из
	// диагностики выбранного хоста.
	snapshot := collect.Snapshot{Time: time.Now(), Servers: []collect.Metrics{
		{Name: "kava", Group: "main", Online: true, Time: time.Now()},
		{Name: "db", Group: "main", Online: true, Time: time.Now(), Docker: collect.DockerCounts{Running: 7, Stopped: 2, Known: true}},
	}}
	m := Model{screen: screenFleet, snapshot: snapshot, fleet: newFleetModel(), layout: newLayout(80, 30)}
	m, _ = updateModel(t, m, key("right"))
	view := stripANSI(m.View())
	if !strings.Contains(view, "DOCKER") {
		t.Fatalf("колонки DOCKER нет в режиме деталей:\n%s", view)
	}
	if row := fleetRowOf(t, view, "db"); !strings.Contains(row, "●7 ○2") {
		t.Fatalf("счётчики контейнеров не дошли до невыбранной строки: %q", row)
	}
}

// fleetRowOf возвращает строку списка, в которой стоит имя хоста. Рамки
// пропускаются: заголовок панели сайдбара тоже подписан именем хоста.
func fleetRowOf(t *testing.T, view, name string) string {
	t.Helper()
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, name+" ") && !strings.ContainsAny(line, "╭╰") {
			return line
		}
	}
	t.Fatalf("строка хоста %q не найдена:\n%s", name, view)
	return ""
}

func TestFleetTableKeepsNumbersAtTheRightEdge(t *testing.T) {
	// Given: широкий терминал с сайдбаром — тот случай из скриншота, где таблица
	// жалась к левому краю панели «СЕРВЕРЫ».
	snapshot := collect.Snapshot{Time: time.Now(), Servers: []collect.Metrics{
		{Name: "kava", Group: "main", Online: true, Time: time.Now(), CPUPct: 4, MemPct: 52, Load1: 0.12},
	}}
	m := Model{screen: screenFleet, snapshot: snapshot, fleet: newFleetModel(), layout: newLayout(200, 30)}

	// When: кадр отрисован.
	view := m.View()

	// Then: строка хоста заканчивается своим последним числом вплотную к рамке.
	var row string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "kava") {
			row = line
		}
	}
	if row == "" {
		t.Fatalf("строка хоста не найдена:\n%s", view)
	}
	if !strings.Contains(stripANSI(row), "0.12 │") {
		t.Fatalf("числа не дошли до правого края панели: %q", stripANSI(row))
	}
}

// TestFleetCardBarsAreFramedAndAligned — Дано: раскрытая карточка с тремя
// шкалами подряд; Тогда: у каждой своя рамка и все три начинаются и кончаются
// в одной колонке — без этого cpu/mem/disk читались как один прямоугольник.
func TestFleetCardBarsAreFramedAndAligned(t *testing.T) {
	server := collect.Metrics{
		Name: "kava", Online: true, Time: time.Now(), Hostname: "kava-claw", NumCPU: 2,
		CPUPct: 4, MemPct: 51, MemTotalKB: 3800000, MemAvailKB: 1800000,
		Disks: []collect.DiskUsage{{Mount: "/", TotalKB: 28000000, UsedKB: 25800000, UsedPct: 92}},
	}
	m := Model{screen: screenFleet, fleet: newFleetModel()}
	for _, width := range []int{60, 100, 160} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			var opens, closes []int
			for _, line := range m.fleetCardLines(server, width) {
				plain := stripANSI(line)
				for _, label := range []string{"cpu", "mem", "disk"} {
					if !strings.Contains(plain, label+" ") || !strings.Contains(plain, "[") {
						continue
					}
					opens = append(opens, columnEnd(plain, "["))
					closes = append(closes, columnEnd(plain, "]"))
				}
			}
			if len(opens) != 3 {
				t.Fatalf("обрамлены не все три шкалы: %v", opens)
			}
			for i := 1; i < 3; i++ {
				if opens[i] != opens[0] || closes[i] != closes[0] {
					t.Fatalf("шкалы разъехались: начала %v, концы %v", opens, closes)
				}
			}
		})
	}
}

func TestFleetTopMemorySurvivesLiveProcessOutput(t *testing.T) {
	// Given: сайдбар флота, ждущий ответа `ps`.
	snapshot := collect.Snapshot{Time: time.Now(), Servers: []collect.Metrics{
		{Name: "kava", Group: "main", Online: true, Time: time.Now(), MemTotalKB: 8 * 1024 * 1024},
	}}
	m := Model{screen: screenFleet, snapshot: snapshot, fleet: newFleetModel(), layout: newLayout(140, 30)}
	m.processes.generation, m.processes.status = 7, diagnosticsLoading

	// When: приходит вывод живого хоста — в нём виден шелл нашей же команды с
	// маркером «утилиты нет» в аргументах.
	raw := " 28841  0.0  0.0 sh -c command -v ps >/dev/null 2>&1 || { echo __SSHMON_UNSUPPORTED__; exit 0; }; ps -eo pid=,pcpu=,pmem=,args=\n" +
		" 28842  0.5  9.4 /usr/bin/java -jar app.jar\n"
	items, err := collect.ParseProcesses(raw)
	if err != nil {
		t.Fatalf("вывод живого ps признан неподдерживаемым: %v", err)
	}
	loaded, _ := updateModel(t, m, processesResultMsg{generation: 7, items: items})

	// Then: раздел показывает процесс, а не «ps недоступен».
	view := loaded.View()
	if !strings.Contains(view, "java") {
		t.Fatalf("сайдбар не показал топ по памяти:\n%s", view)
	}
	for _, unwanted := range []string{"ps недоступен", "нет данных"} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("сайдбар показал %q при живых данных:\n%s", unwanted, view)
		}
	}
}

func TestFleetTopMemoryNamesTheReasonInsteadOfNoData(t *testing.T) {
	// Given: сайдбар флота, чей запрос `ps` сорвался по связи.
	snapshot := collect.Snapshot{Time: time.Now(), Servers: []collect.Metrics{
		{Name: "kava", Group: "main", Online: true, Time: time.Now()},
	}}
	m := Model{screen: screenFleet, snapshot: snapshot, fleet: newFleetModel(), layout: newLayout(140, 30)}
	m.processes.generation, m.processes.status = 7, diagnosticsLoading

	// When: приходит ответ с ошибкой.
	failed, _ := updateModel(t, m, processesResultMsg{generation: 7, err: errors.New("канал закрыт")})

	// Then: в разделе видна причина, а не «нет данных», которое читается как
	// «на хосте нет процессов».
	view := failed.View()
	if !strings.Contains(view, "канал закрыт") || strings.Contains(view, "нет данных") {
		t.Fatalf("причина отказа ps не видна:\n%s", view)
	}
}

func TestFleetNavigationFollowsGroupedDisplayOrder(t *testing.T) {
	// Given interleaved groups so config order differs from display order.
	m := Model{screen: screenFleet, snapshot: collect.Snapshot{Servers: []collect.Metrics{{Name: "alpha", Group: "prod"}, {Name: "bravo", Group: "data"}, {Name: "charlie", Group: "prod"}}}}
	// When moving down from the first displayed server.
	m, _ = updateModel(t, m, tea.KeyMsg{Type: tea.KeyDown})
	// Then selection lands on the next server in grouped display order.
	if m.selected != 2 {
		t.Fatalf("selected=%d, want charlie (2) as next in grouped order", m.selected)
	}
}

func TestFleetWideDrawsTwoBorderedColumns(t *testing.T) {
	// Given a wide Fleet with an online selected server carrying host details.
	now := time.Now().Add(-5 * time.Second)
	snapshot := collect.Snapshot{Time: time.Now(), Servers: []collect.Metrics{{Name: "kava", Group: "main", Online: true, Time: now, Hostname: "kava-claw", CPUPct: 1, MemPct: 44, Load1: 0.03, Uptime: 50 * time.Hour}}}
	m := Model{screen: screenFleet, snapshot: snapshot, config: &config.Config{Servers: []config.Server{{Name: "kava", Host: "10.0.0.1", Group: "main"}}}}
	m.layout = newLayout(140, 40)
	// When the Fleet screen is rendered.
	view := m.View()
	// Then both columns are framed and the right column shows enlarged host details.
	for _, want := range []string{"╭", "╮", "╰", "╯", "СЕРВЕРЫ", "kava-claw", "ЧТО НЕ ТАК"} {
		if !strings.Contains(view, want) {
			t.Fatalf("wide fleet missing %q:\n%s", want, view)
		}
	}
}

func TestFleetKeepsHostListVisibleUnderTheLogDrawer(t *testing.T) {
	// Given: экран флота с группой хостов и открытым ящиком логов на низком
	// терминале — сумма шапки, плиток, ящика и списка в высоту не влезает.
	for _, height := range []int{16, 20} {
		t.Run(strconv.Itoa(height), func(t *testing.T) {
			streamer := &fakeLogStreamer{streams: []collect.LogStream{{
				Lines:  make(chan string, 1),
				Errors: make(chan error, 1),
				Close:  func() error { return nil },
			}}}
			m := Model{
				screen: screenFleet,
				snapshot: collect.Snapshot{Time: time.Now(), Servers: []collect.Metrics{
					{Name: "web", Group: "prod", Online: true, Time: time.Now()},
					{Name: "db", Group: "prod", Online: true, Time: time.Now()},
					{Name: "cache", Group: "prod", Online: true, Time: time.Now()},
				}},
				logSource: streamer,
				logs:      newLogsScreen(),
				fleet:     newFleetModel(),
			}
			m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 100, Height: height})
			m, _ = updateModel(t, m, key("l"))
			for i := range 8 {
				m.logs.buffer.Append(fmt.Sprintf("19:41:0%d info строка", i))
			}

			// When: кадр отрисован.
			view := m.View()

			// Then: на экране остались и ящик, и строка списка, а кадр ровно по
			// высоте терминала.
			if lines := strings.Split(view, "\n"); len(lines) != height {
				t.Fatalf("кадр в %d строк при высоте терминала %d:\n%s", len(lines), height, view)
			}
			if !strings.Contains(view, "ЛОГИ · web") {
				t.Fatalf("ящик логов пропал из кадра:\n%s", view)
			}
			if !strings.Contains(view, fleetMarker) {
				t.Fatalf("под ящиком не осталось ни одной строки списка:\n%s", view)
			}
		})
	}
}

func TestFleetSingleColumnScrollsToTheSelectedRow(t *testing.T) {
	// Given: 28 хостов на терминале, где список в одну колонку и не помещается.
	servers := make([]collect.Metrics, 0, 28)
	for i := range 28 {
		servers = append(servers, collect.Metrics{Name: fmt.Sprintf("host-%02d", i), Online: true, Time: time.Now()})
	}
	m := Model{
		screen:   screenFleet,
		snapshot: collect.Snapshot{Time: time.Now(), Servers: servers},
		fleet:    newFleetModel(),
		selected: 27,
	}
	m, _ = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// When: кадр отрисован.
	view := m.View()

	// Then: список прокручен к выделенной строке, а кадр по высоте терминала.
	if lines := strings.Split(view, "\n"); len(lines) != 24 {
		t.Fatalf("кадр в %d строк при высоте терминала 24:\n%s", len(lines), view)
	}
	if !strings.Contains(view, "host-27") || !strings.Contains(view, fleetMarker) {
		t.Fatalf("выделенная строка уехала за нижний край:\n%s", view)
	}
}

func TestFleetSelectedRowStyleTogglesGreen(t *testing.T) {
	// Given the shared fleet row styles.
	// When choosing the style for the selected versus other rows.
	// Then the cursor row uses the green focus color and others stay dim gray.
	if fleetRowStyle(true).GetForeground() != focusStyle.GetForeground() {
		t.Fatalf("selected row color = %v, want focus %v", fleetRowStyle(true).GetForeground(), focusStyle.GetForeground())
	}
	if fleetRowStyle(false).GetForeground() != dimStyle.GetForeground() {
		t.Fatalf("unselected row color = %v, want dim %v", fleetRowStyle(false).GetForeground(), dimStyle.GetForeground())
	}
}
