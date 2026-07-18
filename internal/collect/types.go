// Package collect — опрос серверов по SSH и агрегация метрик.
package collect

import (
	"fmt"
	"strings"
	"time"
)

type DiskUsage struct {
	Fs      string
	Mount   string
	TotalKB uint64
	UsedKB  uint64
	AvailKB uint64
	UsedPct float64
}

type DiskIO struct {
	Dev      string
	ReadBps  float64
	WriteBps float64
}

type NetRate struct {
	Iface string
	RxBps float64
	TxBps float64
}

type Port struct {
	Proto   string
	Local   string
	Process string
	PID     int
}

type Process struct {
	PID     int
	Command string
	CPUPct  float64
	MemPct  float64
}

type Container struct {
	ID       string
	Name     string
	Image    string
	Status   string
	CPUPct   float64
	MemPct   float64
	MemUsage string
}

type Metrics struct {
	Name     string
	Group    string
	Time     time.Time
	Online   bool
	Err      string
	Hostname string
	Uptime   time.Duration
	Load1    float64
	Load5    float64
	Load15   float64
	NumCPU   int
	CPUPct   float64

	MemTotalKB  uint64
	MemAvailKB  uint64
	SwapTotalKB uint64
	SwapFreeKB  uint64
	MemPct      float64

	Disks []DiskUsage
	IO    []DiskIO
	Net   []NetRate
	Ports []Port
}

type Issue struct {
	Server   string
	Severity string // warn | crit
	Msg      string
}

type Snapshot struct {
	Time       time.Time
	Servers    []Metrics // в порядке конфига
	Issues     []Issue
	HistoryErr string
}

func groupTag(g string) string {
	if g == "" {
		return ""
	}
	return " [" + g + "]"
}

// Text — текстовое резюме снапшота для system prompt LLM-чата.
func (s Snapshot) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Snapshot time: %s\n", s.Time.Format(time.RFC3339))
	for _, m := range s.Servers {
		if m.Time.IsZero() {
			fmt.Fprintf(&b, "\n## %s: данных ещё нет (первый опрос не завершён)\n", m.Name)
			continue
		}
		if !m.Online {
			fmt.Fprintf(&b, "\n## %s: OFFLINE (%s)\n", m.Name, m.Err)
			continue
		}
		fmt.Fprintf(&b, "\n## %s%s (hostname %s), uptime %s\n", m.Name, groupTag(m.Group), m.Hostname, m.Uptime.Round(time.Minute))
		fmt.Fprintf(&b, "cpu: %.0f%% of %d cores, load %.2f %.2f %.2f\n", m.CPUPct, m.NumCPU, m.Load1, m.Load5, m.Load15)
		fmt.Fprintf(&b, "mem: %.0f%% used (%d/%d MB), swap free %d/%d MB\n",
			m.MemPct, (m.MemTotalKB-m.MemAvailKB)/1024, m.MemTotalKB/1024, m.SwapFreeKB/1024, m.SwapTotalKB/1024)
		for _, d := range m.Disks {
			fmt.Fprintf(&b, "disk %s (%s): %.0f%% used, %d/%d MB\n", d.Mount, d.Fs, d.UsedPct, d.UsedKB/1024, d.TotalKB/1024)
		}
		for _, io := range m.IO {
			fmt.Fprintf(&b, "io %s: read %.0f B/s, write %.0f B/s\n", io.Dev, io.ReadBps, io.WriteBps)
		}
		for _, n := range m.Net {
			fmt.Fprintf(&b, "net %s: rx %.0f B/s, tx %.0f B/s\n", n.Iface, n.RxBps, n.TxBps)
		}
		for i, p := range m.Ports {
			if i >= 15 {
				fmt.Fprintf(&b, "... и ещё %d портов\n", len(m.Ports)-15)
				break
			}
			fmt.Fprintf(&b, "port %s %s %s\n", p.Proto, p.Local, p.Process)
		}
	}
	if len(s.Issues) > 0 {
		b.WriteString("\n## Issues\n")
		for _, is := range s.Issues {
			fmt.Fprintf(&b, "- [%s] %s: %s\n", is.Severity, is.Server, is.Msg)
		}
	} else {
		b.WriteString("\nNo issues detected.\n")
	}
	return b.String()
}
