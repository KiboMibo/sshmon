package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kibomibo/sshmon/internal/collect"
)

type processSort uint8

const (
	processSortCPU processSort = iota
	processSortMemory
	processSortPID
	processSortName
)

type processScreen struct {
	items      []collect.Process
	sort       processSort
	status     diagnosticsStatus
	err        error
	generation uint64
	cancel     func()
}

func sortProcesses(items []collect.Process, by processSort) []collect.Process {
	result := append([]collect.Process(nil), items...)
	sort.SliceStable(result, func(i, j int) bool {
		a, b := result[i], result[j]
		switch by {
		case processSortMemory:
			if a.MemPct != b.MemPct {
				return a.MemPct > b.MemPct
			}
		case processSortPID:
			if a.PID != b.PID {
				return a.PID < b.PID
			}
		case processSortName:
			if a.Command != b.Command {
				return a.Command < b.Command
			}
		default:
			if a.CPUPct != b.CPUPct {
				return a.CPUPct > b.CPUPct
			}
		}
		return a.PID < b.PID
	})
	return result
}

func (s *processScreen) apply(items []collect.Process, err error) {
	if err == nil {
		s.items = append([]collect.Process(nil), items...)
	}
	s.err = err
	s.status = diagnosticsResultStatus(err, len(s.items) > 0)
}

func (m Model) renderProcesses() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("sshmon · "+m.selectedName()+" · Процессы") + "\n\n")
	b.WriteString("PID     CPU     MEM     КОМАНДА\n")
	for _, p := range sortProcesses(m.processes.items, m.processes.sort) {
		b.WriteString(fmt.Sprintf("%-7d %6.1f%% %6.1f%%  %s\n", p.PID, p.CPUPct, p.MemPct, p.Command))
	}
	b.WriteString("\n" + dimStyle.Render(diagnosticsFooter(m.processes.status, m.processes.err)))
	return b.String()
}

func (m Model) diagnosticsGeneration(screen screenKind) uint64 {
	switch screen {
	case screenProcesses:
		return m.processes.generation
	case screenPorts:
		return m.ports.generation
	case screenContainers:
		return m.containers.generation
	default:
		return 0
	}
}
