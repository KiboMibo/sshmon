package collect

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kibomibo/sshmon/internal/config"
	"github.com/kibomibo/sshmon/internal/sshx"
)

type serverState struct {
	cfg  config.Server
	cli  *sshx.Client
	m    Metrics
	prev *counters
}

type Collector struct {
	cfg    *config.Config
	mu     sync.Mutex
	states []*serverState
}

func New(cfg *config.Config) *Collector {
	c := &Collector{cfg: cfg}
	for _, s := range cfg.Servers {
		c.states = append(c.states, &serverState{cfg: s, cli: sshx.New(s), m: Metrics{Name: s.Name}})
	}
	return c
}

// Run опрашивает все серверы раз в interval до отмены контекста.
func (c *Collector) Run(ctx context.Context) {
	c.pollAll()
	t := time.NewTicker(c.cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.pollAll()
		}
	}
}

func (c *Collector) pollAll() {
	var wg sync.WaitGroup
	for _, st := range c.states {
		wg.Add(1)
		go func(st *serverState) {
			defer wg.Done()
			c.poll(st)
		}(st)
	}
	wg.Wait()
}

func (c *Collector) poll(st *serverState) {
	now := time.Now()
	raw, err := st.cli.Run(sampleCmd, 15*time.Second)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		st.m = Metrics{Name: st.cfg.Name, Time: now, Online: false, Err: err.Error()}
		st.prev = nil
		return
	}
	s := parseSample(raw, now)
	m := Metrics{
		Name: st.cfg.Name, Time: now, Online: true,
		Hostname: s.hostname, Uptime: s.uptime,
		Load1: s.load1, Load5: s.load5, Load15: s.load15,
		NumCPU:     s.c.ncpu,
		MemTotalKB: s.memTotal, MemAvailKB: s.memAvail,
		SwapTotalKB: s.swapTot, SwapFreeKB: s.swapFree,
		Disks: s.disks, Ports: s.ports,
	}
	if m.MemTotalKB > 0 {
		m.MemPct = 100 * float64(m.MemTotalKB-m.MemAvailKB) / float64(m.MemTotalKB)
	}
	if st.prev != nil {
		m.CPUPct, m.IO, m.Net = rates(st.prev, &s.c)
	}
	st.prev = &s.c
	st.m = m
}

// Snapshot — копия текущего состояния всех серверов + детекция проблем.
// Слайсы внутри Metrics разделяются с внутренним состоянием: только чтение.
func (c *Collector) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := Snapshot{Time: time.Now()}
	for _, st := range c.states {
		s.Servers = append(s.Servers, st.m)
	}
	s.Issues = c.issuesLocked(s.Servers)
	return s
}

func (c *Collector) issuesLocked(ms []Metrics) []Issue {
	thr := c.cfg.Thresholds
	sev := func(v float64) string {
		if v >= 97 {
			return "crit"
		}
		return "warn"
	}
	var out []Issue
	for _, m := range ms {
		if m.Time.IsZero() {
			continue // ещё не опрошен
		}
		if !m.Online {
			out = append(out, Issue{m.Name, "crit", "недоступен: " + m.Err})
			continue
		}
		if m.CPUPct >= thr.CPU {
			out = append(out, Issue{m.Name, sev(m.CPUPct), fmt.Sprintf("CPU %.0f%%", m.CPUPct)})
		}
		if m.MemPct >= thr.Mem {
			out = append(out, Issue{m.Name, sev(m.MemPct), fmt.Sprintf("память %.0f%%", m.MemPct)})
		}
		for _, d := range m.Disks {
			if d.UsedPct >= thr.Disk {
				out = append(out, Issue{m.Name, sev(d.UsedPct), fmt.Sprintf("диск %s заполнен на %.0f%%", d.Mount, d.UsedPct)})
			}
		}
	}
	return out
}

// TailLog возвращает последние строки логов сервера
// (journalctl → syslog → messages → logread для BusyBox/OpenWrt).
func (c *Collector) TailLog(server string, lines int) (string, error) {
	var cli *sshx.Client
	c.mu.Lock()
	for _, st := range c.states {
		if st.cfg.Name == server {
			cli = st.cli
		}
	}
	c.mu.Unlock()
	if cli == nil {
		return "", fmt.Errorf("неизвестный сервер %q", server)
	}
	if lines <= 0 {
		lines = 200
	}
	cmd := fmt.Sprintf(
		"journalctl -n %d --no-pager 2>/dev/null || tail -n %d /var/log/syslog 2>/dev/null || tail -n %d /var/log/messages 2>/dev/null || (logread 2>/dev/null | tail -n %d)",
		lines, lines, lines, lines)
	return cli.Run(cmd, 20*time.Second)
}
