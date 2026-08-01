package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/kibomibo/sshmon/internal/collect"
	"github.com/kibomibo/sshmon/internal/config"
)

// serverScreenModel — экран сервера со всеми блоками макета: метрики, docker,
// сервисы, порты и логи. Размер терминала задаётся вызывающим тестом.
func serverScreenModel(width, height int) Model {
	m := dashboardWorkspaceFixture()
	server := m.snapshot.Servers[0]
	server.OS = "debian 12"
	server.SwapTotalKB, server.SwapFreeKB = 2<<20, 1<<20
	server.Disks = []collect.DiskUsage{{Fs: "/dev/sda2", Mount: "/", TotalKB: 49 << 20, UsedKB: 38 << 20, UsedPct: 78}}
	server.Ports = []collect.Port{
		{Proto: "tcp", Local: "10.2.4.18:22", Process: "sshd"},
		{Proto: "tcp", Local: "10.2.4.18:5432", Process: "postgres"},
	}
	m.snapshot.Servers[0] = server
	m.dashboard.containers = dashboardContainersState{status: diagnosticsReady, items: []collect.Container{
		{Name: "pg-main", Status: "Up 20 weeks", RunningFor: "20 weeks ago", MemUsage: "6.1GiB / 15.6GiB"},
		{Name: "mailhog", Status: "Exited (0) 12 days ago", RunningFor: "12 days ago", MemUsage: "0B / 0B"},
	}}
	m.layout = newLayout(width, height)
	return m
}

func TestServerScreenKeepsEveryBlockAtSupportedSizes(t *testing.T) {
	// Given: экран сервера с полным набором данных на всех поддерживаемых
	// размерах, включая минимальный 60×16 и загнанный в compact 60×18
	// (плитка ПРОБЛЕМЫ съедает высоту, бюджета тела меньше семи строк).
	for _, size := range []struct {
		width, height int
		issues        bool
	}{{width: 60, height: 16}, {width: 60, height: 18, issues: true}, {width: 80, height: 24}, {width: 100, height: 16}, {width: 100, height: 20}, {width: 120, height: 30}, {width: 160, height: 50}} {
		m := serverScreenModel(size.width, size.height)
		if size.issues {
			m.snapshot.Issues = []collect.Issue{{Server: m.snapshot.Servers[0].Name, Severity: "crit",
				Msg: "недоступен: host-key сервера не совпадает с записью в known_hosts — выполните ssh-keygen -R и переподключитесь вручную"}}
		}

		// When: экран отрисован целиком.
		view := m.View()
		lines := strings.Split(view, "\n")

		// Then: кадр ровно по терминалу и ни одна строка не шире него.
		if len(lines) != size.height {
			t.Fatalf("%dx%d: %d строк, ожидалось %d:\n%s", size.width, size.height, len(lines), size.height, view)
		}
		for i, line := range lines {
			if got := lipgloss.Width(line); got > size.width {
				t.Fatalf("%dx%d: строка %d шириной %d:\n%q", size.width, size.height, i, got, line)
			}
		}
		// Then: сетка метрик, docker, сервисы и логи на месте при любом размере.
		for _, want := range []string{"CPU", "MEM", "NET", "DISK", "DOCKER", "СЕРВИСЫ", "ЛОГИ"} {
			if !strings.Contains(view, want) {
				t.Fatalf("%dx%d: экран без блока %q:\n%s", size.width, size.height, want, view)
			}
		}
		// Then: свёрнутый блок помечен признаком усечения, а не потерян молча.
		if !strings.Contains(view, "ПОРТЫ") && !strings.Contains(view, "усечено") {
			t.Fatalf("%dx%d: ПОРТЫ пропали без признака усечения:\n%s", size.width, size.height, view)
		}
	}
}

func TestServerScreenMetricGridSharesOneColumnLayout(t *testing.T) {
	// Given: сервер с загрузкой, памятью, сетью и диском.
	m := serverScreenModel(120, 30)

	// When: рисуется сетка метрик.
	rows := serverMetricGrid(m.snapshot.Servers[0], nil, 120)

	// Then: четыре однородные строки с деталями макета на одной сетке.
	if len(rows) != 4 {
		t.Fatalf("сетка из %d строк, ожидалось 4: %#v", len(rows), rows)
	}
	for index, want := range []string{"CPU", "MEM", "NET", "DISK"} {
		if !strings.HasPrefix(rows[index], want) {
			t.Fatalf("строка %d = %q, ожидалась метка %q", index, rows[index], want)
		}
	}
	for _, want := range []string{"load 0.40", "swap", "rx", "tx", "78%", "/ (sda2)"} {
		if !strings.Contains(strings.Join(rows, "\n"), want) {
			t.Fatalf("сетка без %q:\n%s", want, strings.Join(rows, "\n"))
		}
	}
	// Then: у NET колонка процента пустая, но детали стоят там же, где у прочих.
	if strings.Contains(rows[2], "%") {
		t.Fatalf("у NET не должно быть процента: %q", rows[2])
	}
	if runeIndexOf(rows[2], "rx") != runeIndexOf(rows[0], "load") {
		t.Fatalf("детали NET и CPU разъехались:\n%q\n%q", rows[2], rows[0])
	}
}

func TestServerHeaderCarriesFactsAndFleetWording(t *testing.T) {
	// Given: доступный сервер с дистрибутивом и аптаймом.
	m := serverScreenModel(120, 30)
	server := m.snapshot.Servers[0]
	server.Uptime = 140 * 24 * time.Hour
	m.snapshot.Servers[0] = server

	// When: рисуется шапка экрана.
	header := m.serverHeader(m.snapshot.Servers[0], 120)

	// Then: состояние названо как на флоте, а факты — по макету.
	for _, want := range []string{"● норма", "web-01", "debian 12", "4 ядер", "up 140д"} {
		if !strings.Contains(header, want) {
			t.Fatalf("шапка без %q: %q", want, header)
		}
	}
	// Then: свежесть данных ушла из шапки в статусбар.
	if strings.Contains(header, "данные") {
		t.Fatalf("свежесть данных осталась в шапке: %q", header)
	}
	if !strings.Contains(m.serverFooter(m.snapshot.Servers[0], 120, false), "данные") {
		t.Fatal("свежесть данных потеряна: её нет и в статусбаре")
	}
}

// TestServerHeaderKeepsFactsOnMinimalWidth — Дано: терминал в минимальные 60
// колонок; Когда: рисуется шапка; Тогда: факты остаются видны хотя бы
// усечёнными, а адрес из конфига стоит вместо hostname (макет 3c).
func TestServerHeaderKeepsFactsOnMinimalWidth(t *testing.T) {
	// Дано: сервер с адресом в конфиге и полным набором фактов макета —
	// вместе с именем и состоянием они заведомо не помещаются в 60 колонок.
	m := serverScreenModel(60, 16)
	m.config = &config.Config{Servers: []config.Server{{Name: m.snapshot.Servers[0].Name, Host: "10.2.4.18"}}}
	server := m.snapshot.Servers[0]
	server.Group, server.Uptime = "vm", 140*24*time.Hour
	m.snapshot.Servers[0] = server

	// Когда: шапка рисуется в минимальную поддерживаемую ширину.
	header := m.serverHeader(m.snapshot.Servers[0], 60)

	// Тогда: кадр не переполнен, а правая часть не исчезла целиком.
	if got := lipgloss.Width(header); got > 60 {
		t.Fatalf("шапка шириной %d на 60 колонках: %q", got, header)
	}
	for _, want := range []string{"web", "● норма", "10.2.4.18", "debian 12"} {
		if !strings.Contains(header, want) {
			t.Fatalf("шапка на 60 колонках без %q: %q", want, header)
		}
	}
	// Тогда: hostname уступил адрес — по нему сервер и находят.
	if strings.Contains(header, "web-01") {
		t.Fatalf("hostname вытеснил адрес: %q", header)
	}
	// И: без конфига адреса нет — hostname остаётся запасным вариантом.
	m.config = nil
	if !strings.Contains(m.serverHeader(m.snapshot.Servers[0], 60), "web-01") {
		t.Fatalf("без адреса hostname должен остаться: %q", m.serverHeader(m.snapshot.Servers[0], 60))
	}
}

// TestServerScreenKeepsPortsOnTightBudget — Дано: терминалы, где под тело
// экрана остаётся 10–11 строк; Когда: экран собран; Тогда: ПОРТЫ помещаются,
// а логам всё равно достаётся их минимум.
func TestServerScreenKeepsPortsOnTightBudget(t *testing.T) {
	// Дано: высоты 18–20 дают бюджет тела 10–12 строк.
	for _, height := range []int{18, 19, 20} {
		view := serverScreenModel(100, height).View()

		// Тогда: плитка ПОРТЫ на месте и признака усечения нет.
		if !strings.Contains(view, "ПОРТЫ") || strings.Contains(view, "усечено") {
			t.Fatalf("100×%d: ПОРТЫ не поместились:\n%s", height, view)
		}
		// Тогда: логи не ужаты ниже своего минимума — заголовок и строка лога на месте.
		if !strings.Contains(view, "ЛОГИ") || !strings.Contains(view, "system ready") {
			t.Fatalf("100×%d: логи потеряли строки:\n%s", height, view)
		}
	}
	// И: на бюджете 9 строк (100×17) пара плиток и минимум логов уже не
	// помещаются — ПОРТЫ уходят, но под признаком усечения, а не молча.
	view := serverScreenModel(100, 17).View()
	if strings.Contains(view, "ПОРТЫ") == strings.Contains(view, "усечено") {
		t.Fatalf("100×17: усечение не отмечено:\n%s", view)
	}
}

// TestLongIssueListKeepsMetricGridAndLogs — Дано: длинный список проблем;
// Когда: экран собран; Тогда: плитка ПРОБЛЕМЫ ограничена по высоте и не
// срезает сетку метрик, а неполнота списка видна счётчиком.
func TestLongIssueListKeepsMetricGridAndLogs(t *testing.T) {
	// Дано: восемь развёрнутых проблем — свободного места им заведомо не хватит.
	m := serverScreenModel(100, 24)
	for index := range 8 {
		m.snapshot.Issues = append(m.snapshot.Issues, collect.Issue{
			Server: m.snapshot.Servers[0].Name, Severity: "warn",
			Msg: fmt.Sprintf("диск /var/lib/postgresql/%d заполнен на 97%% — осталось 1.2G из 49G", index),
		})
	}

	// Когда: экран отрисован.
	view := m.View()
	lines := strings.Split(view, "\n")

	// Тогда: кадр ровно по терминалу, а сетка метрик и логи на месте.
	if len(lines) != 24 {
		t.Fatalf("%d строк, ожидалось 24:\n%s", len(lines), view)
	}
	for _, want := range []string{"ПРОБЛЕМЫ", "CPU", "MEM", "NET", "DISK", "ЛОГИ"} {
		if !strings.Contains(view, want) {
			t.Fatalf("длинный список проблем съел блок %q:\n%s", want, view)
		}
	}
	// Тогда: плитка проблем не выше потолка, а остаток списка назван числом.
	if !strings.Contains(view, "ещё") {
		t.Fatalf("усечение списка проблем не отмечено:\n%s", view)
	}
	if got := issueRows(m.dashboardIssueText(m.snapshot.Servers[0].Name), 96); len(got) != maxIssueRows {
		t.Fatalf("плитка проблем из %d строк, ожидалось %d: %#v", len(got), maxIssueRows, got)
	}
}

func TestServerScreenShowsIssueAndKeepsLastMetricsWhenOffline(t *testing.T) {
	// Given: сервер недоступен, но последние метрики двухминутной давности есть.
	now := time.Date(2026, 7, 18, 22, 0, 0, 0, time.UTC)
	m := Model{
		screen: screenDashboard,
		layout: newLayout(100, 30),
		snapshot: collect.Snapshot{Time: now, Servers: []collect.Metrics{{
			Name: "db", Hostname: "db-01", Online: false, Err: "dial timeout", Time: now.Add(-2 * time.Minute),
			CPUPct: 38, MemPct: 55, Load1: 0.7,
		}}, Issues: []collect.Issue{{Server: "db", Severity: "crit", Msg: "недоступен: dial timeout"}}},
	}
	m.snapshot.HistoryErr = "database locked"

	// When: экран отрисован.
	view := m.View()

	// Then: видно состояние, проблему, последние значения и их возраст.
	for _, want := range []string{"× недоступен", "ПРОБЛЕМЫ", "dial timeout", "переподключить", "38%", "55%", "данные 2m"} {
		if !strings.Contains(view, want) {
			t.Fatalf("экран offline-сервера без %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "сервер недоступен: database locked") {
		t.Fatalf("ошибка истории подана как отказ сервера:\n%s", view)
	}
}
