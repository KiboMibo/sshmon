// Package tui — интерактивный интерфейс на bubbletea:
// вкладки Overview / Detail / Ports / Logs / Chat.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kibomibo/sshmon/internal/collect"
	"github.com/kibomibo/sshmon/internal/config"
	"github.com/kibomibo/sshmon/internal/llm"
)

const (
	tabOverview = iota
	tabDetail
	tabPorts
	tabLogs
	tabChat
	tabCount
)

var tabNames = []string{"Overview", "Detail", "Ports", "Logs", "Chat"}

var (
	styActive = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Underline(true)
	styTab    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	styGood   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	styWarn   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styCrit   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	styDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	styTitle  = lipgloss.NewStyle().Bold(true)
)

type tickMsg time.Time
type logsMsg struct {
	server, text string
	err          error
}
type chatMsg struct {
	text string
	err  error
}

type Model struct {
	col *collect.Collector
	llm *llm.Client
	cfg *config.Config

	tab  int
	sel  int
	snap collect.Snapshot

	logs     map[string]string
	logsBusy bool
	logsVP   viewport.Model

	chatVP   viewport.Model
	input    textinput.Model
	chat     []llm.Message
	chatBusy bool
	chatErr  string

	w, h  int
	ready bool
}

func New(col *collect.Collector, lc *llm.Client, cfg *config.Config) Model {
	ti := textinput.New()
	ti.Placeholder = "Спросить о серверах… (Enter — отправить, Esc — выйти из ввода)"
	ti.CharLimit = 4000
	return Model{col: col, llm: lc, cfg: cfg, logs: map[string]string{}, input: ti}
}

func (m Model) Init() tea.Cmd { return tick() }

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		bodyH := m.h - 4
		if bodyH < 3 {
			bodyH = 3
		}
		if !m.ready {
			m.logsVP = viewport.New(m.w, bodyH)
			m.chatVP = viewport.New(m.w, bodyH-2)
			m.ready = true
		} else {
			m.logsVP.Width, m.logsVP.Height = m.w, bodyH
			m.chatVP.Width, m.chatVP.Height = m.w, bodyH-2
		}
		m.input.Width = m.w - 6
		m.refreshViews()
		return m, nil

	case tickMsg:
		m.snap = m.col.Snapshot()
		if m.sel >= len(m.snap.Servers) {
			m.sel = 0
		}
		return m, tick()

	case logsMsg:
		m.logsBusy = false
		if msg.err != nil {
			m.logs[msg.server] = "ошибка: " + msg.err.Error()
		} else {
			m.logs[msg.server] = msg.text
		}
		m.refreshViews()
		m.logsVP.GotoBottom()
		return m, nil

	case chatMsg:
		m.chatBusy = false
		if msg.err != nil {
			m.chatErr = msg.err.Error()
		} else {
			m.chatErr = ""
			m.chat = append(m.chat, llm.Message{Role: "assistant", Content: msg.text})
		}
		m.refreshViews()
		m.chatVP.GotoBottom()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.tab == tabChat && m.input.Focused() {
		switch k.String() {
		case "esc":
			m.input.Blur()
			return m, nil
		case "ctrl+c":
			return m, tea.Quit
		case "enter":
			cmd := (&m).sendChat()
			m.refreshViews()
			m.chatVP.GotoBottom()
			return m, cmd
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(k)
			return m, cmd
		}
	}

	switch k.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab":
		m.tab = (m.tab + 1) % tabCount
		return m.enterTab()
	case "shift+tab":
		m.tab = (m.tab + tabCount - 1) % tabCount
		return m.enterTab()
	case "1", "2", "3", "4", "5":
		m.tab = int(k.String()[0] - '1')
		return m.enterTab()
	case "up", "k":
		if m.sel > 0 {
			m.sel--
		}
		return m.afterSelChange()
	case "down", "j":
		if m.sel < len(m.snap.Servers)-1 {
			m.sel++
		}
		return m.afterSelChange()
	case "r":
		if m.tab == tabLogs {
			return m, (&m).fetchLogs()
		}
		return m, nil
	case "i", "enter":
		if m.tab == tabChat {
			m.input.Focus()
			return m, textinput.Blink
		}
	}

	var cmd tea.Cmd
	switch m.tab {
	case tabLogs:
		m.logsVP, cmd = m.logsVP.Update(k)
	case tabChat:
		m.chatVP, cmd = m.chatVP.Update(k)
	}
	return m, cmd
}

func (m Model) enterTab() (tea.Model, tea.Cmd) {
	m.refreshViews()
	if m.tab == tabLogs {
		if name := m.selName(); name != "" && m.logs[name] == "" && !m.logsBusy {
			return m, (&m).fetchLogs()
		}
	}
	if m.tab == tabChat {
		m.input.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

func (m Model) afterSelChange() (tea.Model, tea.Cmd) {
	m.refreshViews()
	if m.tab == tabLogs {
		if name := m.selName(); name != "" && m.logs[name] == "" && !m.logsBusy {
			return m, (&m).fetchLogs()
		}
	}
	return m, nil
}

func (m *Model) refreshViews() {
	if !m.ready {
		return
	}
	switch m.tab {
	case tabLogs:
		m.logsVP.SetContent(m.logs[m.selName()])
	case tabChat:
		m.chatVP.SetContent(m.renderChat())
	}
}

func (m Model) selName() string {
	if m.sel < len(m.snap.Servers) {
		return m.snap.Servers[m.sel].Name
	}
	return ""
}

func (m *Model) fetchLogs() tea.Cmd {
	name := m.selName()
	if name == "" || m.logsBusy {
		return nil
	}
	m.logsBusy = true
	col := m.col
	return func() tea.Msg {
		text, err := col.TailLog(name, 300)
		return logsMsg{name, text, err}
	}
}

func (m *Model) sendChat() tea.Cmd {
	q := strings.TrimSpace(m.input.Value())
	if q == "" || m.chatBusy {
		return nil
	}
	m.input.SetValue("")
	m.chat = append(m.chat, llm.Message{Role: "user", Content: q})
	m.chatBusy = true
	msgs := append([]llm.Message(nil), m.chat...)
	sys := systemPrompt(m.col.Snapshot())
	lc := m.llm
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		text, err := lc.Chat(ctx, sys, msgs)
		return chatMsg{text, err}
	}
}

func systemPrompt(s collect.Snapshot) string {
	return "Ты — ассистент по мониторингу серверов sshmon. Ниже актуальное состояние всех серверов, собранное по SSH. " +
		"Отвечай кратко и по делу, на языке пользователя. Если есть проблемы — объясни вероятную причину и что проверить.\n\n" + s.Text()
}

// ---------- рендеринг ----------

func (m Model) View() string {
	if !m.ready {
		return "загрузка…"
	}
	var b strings.Builder
	b.WriteString(m.renderTabs() + "\n")
	switch m.tab {
	case tabOverview:
		b.WriteString(m.renderOverview())
	case tabDetail:
		b.WriteString(m.renderDetail())
	case tabPorts:
		b.WriteString(m.renderPorts())
	case tabLogs:
		b.WriteString(m.renderLogs())
	case tabChat:
		b.WriteString(m.renderChatTab())
	}
	b.WriteString("\n" + m.renderFooter())
	return b.String()
}

func (m Model) renderTabs() string {
	var parts []string
	for i, n := range tabNames {
		label := fmt.Sprintf(" %d:%s ", i+1, n)
		if i == m.tab {
			parts = append(parts, styActive.Render(label))
		} else {
			parts = append(parts, styTab.Render(label))
		}
	}
	left := strings.Join(parts, " ")
	right := styDim.Render(m.snap.Time.Format("15:04:05"))
	pad := m.w - lipgloss.Width(left) - lipgloss.Width(right) - 1
	if pad < 1 {
		pad = 1
	}
	return left + strings.Repeat(" ", pad) + right
}

func (m Model) renderFooter() string {
	help := "1-5/tab — вкладки · ↑↓ — сервер · r — обновить логи · q — выход"
	if m.tab == tabChat {
		help = "esc — из ввода · tab — вкладки · ctrl+c — выход"
	}
	return styDim.Render(help)
}

func (m Model) renderOverview() string {
	thr := m.cfg.Thresholds
	hasGroups := false
	for _, s := range m.snap.Servers {
		if s.Group != "" {
			hasGroups = true
			break
		}
	}
	gcol := func(g string) string {
		if !hasGroups {
			return ""
		}
		return padR(g, 10) + " "
	}
	var b strings.Builder
	b.WriteString(styDim.Render("  "+padR("СЕРВЕР", 14)+" "+gcol("ГРУППА")+padR("СТАТУС", 6)+" "+padL("CPU", 6)+" "+padL("MEM", 6)+" "+padL("DISK", 6)+" "+padL("NET rx/tx", 16)+" "+padL("LOAD1", 7)+"  ПРОБЛЕМЫ") + "\n")
	for i, s := range m.snap.Servers {
		cur := "  "
		if i == m.sel {
			cur = "▶ "
		}
		if s.Time.IsZero() {
			b.WriteString(cur + padR(s.Name, 14) + " " + gcol(s.Group) + styDim.Render("опрос…") + "\n")
			continue
		}
		if !s.Online {
			b.WriteString(cur + padR(s.Name, 14) + " " + gcol(s.Group) + styCrit.Render(padR("down", 6)+" "+truncate(s.Err, m.w-30)) + "\n")
			continue
		}
		var rx, tx float64
		for _, n := range s.Net {
			rx += n.RxBps
			tx += n.TxBps
		}
		var dmax float64
		for _, d := range s.Disks {
			if d.UsedPct > dmax {
				dmax = d.UsedPct
			}
		}
		nIssues := 0
		for _, is := range m.snap.Issues {
			if is.Server == s.Name {
				nIssues++
			}
		}
		issueStr := styDim.Render("-")
		if nIssues > 0 {
			issueStr = styWarn.Render(fmt.Sprintf("%d", nIssues))
		}
		b.WriteString(cur + padR(s.Name, 14) + " " + gcol(s.Group) + styGood.Render(padR("up", 6)) + " " +
			pctCell(s.CPUPct, thr.CPU) + " " + pctCell(s.MemPct, thr.Mem) + " " + pctCell(dmax, thr.Disk) + " " +
			padL(fmtBytes(rx)+"/"+fmtBytes(tx), 16) + " " + padL(fmt.Sprintf("%.2f", s.Load1), 7) + "  " + issueStr + "\n")
	}
	if len(m.snap.Issues) > 0 {
		b.WriteString("\n" + styTitle.Render("Проблемы:") + "\n")
		for _, is := range m.snap.Issues {
			sty := styWarn
			if is.Severity == "crit" {
				sty = styCrit
			}
			b.WriteString("  " + sty.Render(fmt.Sprintf("[%s] %s: %s", is.Severity, is.Server, is.Msg)) + "\n")
		}
	}
	return b.String()
}

func (m Model) renderDetail() string {
	if len(m.snap.Servers) == 0 {
		return "нет серверов"
	}
	s := m.snap.Servers[m.sel]
	sc := m.cfg.Servers[m.sel]
	thr := m.cfg.Thresholds
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s\n", styTitle.Render(s.Name), styDim.Render(fmt.Sprintf("%s@%s", sc.User, sc.Addr())))
	if s.Time.IsZero() {
		return b.String() + styDim.Render("данных ещё нет…")
	}
	if !s.Online {
		return b.String() + styCrit.Render("недоступен: "+s.Err)
	}
	fmt.Fprintf(&b, "хост: %s   аптайм: %s\n\n", s.Hostname, s.Uptime.Round(time.Minute))
	fmt.Fprintf(&b, "CPU:  %s из %d ядер   load: %.2f %.2f %.2f\n", pctCell(s.CPUPct, thr.CPU), s.NumCPU, s.Load1, s.Load5, s.Load15)
	fmt.Fprintf(&b, "RAM:  %s   %s / %s\n", pctCell(s.MemPct, thr.Mem),
		fmtBytes(float64(s.MemTotalKB-s.MemAvailKB)*1024), fmtBytes(float64(s.MemTotalKB)*1024))
	if s.SwapTotalKB > 0 {
		fmt.Fprintf(&b, "Swap: %s свободно из %s\n", fmtBytes(float64(s.SwapFreeKB)*1024), fmtBytes(float64(s.SwapTotalKB)*1024))
	}
	b.WriteString("\n" + styTitle.Render("Диски:") + "\n")
	for _, d := range s.Disks {
		fmt.Fprintf(&b, "  %s %s  %s  %s / %s  (%s)\n", pctCell(d.UsedPct, thr.Disk), padR(d.Mount, 20),
			padR(d.Fs, 18), fmtBytes(float64(d.UsedKB)*1024), fmtBytes(float64(d.TotalKB)*1024), fmtBytes(float64(d.AvailKB)*1024)+" свободно")
	}
	if len(s.IO) > 0 {
		b.WriteString("\n" + styTitle.Render("Диск IO:") + "\n")
		for _, io := range s.IO {
			fmt.Fprintf(&b, "  %s  R %s/s  W %s/s\n", padR(io.Dev, 12), fmtBytes(io.ReadBps), fmtBytes(io.WriteBps))
		}
	}
	if len(s.Net) > 0 {
		b.WriteString("\n" + styTitle.Render("Сеть:") + "\n")
		for _, n := range s.Net {
			fmt.Fprintf(&b, "  %s  rx %s/s  tx %s/s\n", padR(n.Iface, 12), fmtBytes(n.RxBps), fmtBytes(n.TxBps))
		}
	}
	return b.String()
}

func (m Model) renderPorts() string {
	if len(m.snap.Servers) == 0 {
		return "нет серверов"
	}
	s := m.snap.Servers[m.sel]
	var b strings.Builder
	b.WriteString(styTitle.Render("Порты: "+s.Name) + "\n")
	if !s.Online {
		return b.String() + styCrit.Render("недоступен")
	}
	if len(s.Ports) == 0 {
		return b.String() + styDim.Render("нет данных (ss/netstat не найдены на сервере?)")
	}
	b.WriteString(styDim.Render("  "+padR("ПРОТО", 6)+" "+padR("АДРЕС", 32)+" ПРОЦЕСС") + "\n")
	for _, p := range s.Ports {
		proc := p.Process
		if proc == "" {
			proc = styDim.Render("?")
		}
		b.WriteString("  " + padR(p.Proto, 6) + " " + padR(p.Local, 32) + " " + proc + "\n")
	}
	return b.String()
}

func (m Model) renderLogs() string {
	head := styTitle.Render("Логи: " + m.selName())
	if m.logsBusy {
		head += styDim.Render("  загрузка…")
	} else {
		head += styDim.Render("  (r — обновить)")
	}
	return head + "\n" + m.logsVP.View()
}

func (m Model) renderChatTab() string {
	out := m.chatVP.View() + "\n"
	if m.chatErr != "" {
		out += styCrit.Render("ошибка: "+truncate(m.chatErr, m.w-10)) + "\n"
	}
	status := ""
	if m.chatBusy {
		status = styDim.Render("  думает…")
	}
	return out + m.input.View() + status
}

func (m Model) renderChat() string {
	w := m.w - 2
	if w < 10 {
		w = 10
	}
	wrap := lipgloss.NewStyle().Width(w)
	var b strings.Builder
	if !m.llm.Configured() {
		b.WriteString(styDim.Render("LLM не настроен — заполните секцию llm в конфиге.") + "\n")
	}
	for _, msg := range m.chat {
		who := styGood.Render("Вы:")
		if msg.Role == "assistant" {
			who = styWarn.Render("ИИ:")
		}
		b.WriteString(who + "\n" + wrap.Render(msg.Content) + "\n\n")
	}
	return b.String()
}

// ---------- хелперы ----------

func padR(s string, n int) string {
	if len(s) > n {
		s = s[:n]
	}
	return fmt.Sprintf("%-*s", n, s)
}

func padL(s string, n int) string { return fmt.Sprintf("%*s", n, s) }

func pctCell(v, thr float64) string {
	s := fmt.Sprintf("%5.0f%%", v)
	switch {
	case v >= thr:
		return styCrit.Render(s)
	case v >= thr-15:
		return styWarn.Render(s)
	default:
		return s
	}
}

func fmtBytes(b float64) string {
	units := []string{"B", "K", "M", "G", "T"}
	i := 0
	for b >= 1024 && i < len(units)-1 {
		b /= 1024
		i++
	}
	return fmt.Sprintf("%.1f%s", b, units[i])
}

func truncate(s string, n int) string {
	if n < 3 {
		n = 3
	}
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
