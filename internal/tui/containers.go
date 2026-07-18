package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kibomibo/sshmon/internal/collect"
)

type containerSort uint8

const (
	containerSortCPU containerSort = iota
	containerSortMemory
	containerSortName
)

type containerScreen struct {
	items      []collect.Container
	sort       containerSort
	status     diagnosticsStatus
	err        error
	generation uint64
	cancel     func()
}

func sortContainers(items []collect.Container, by containerSort) []collect.Container {
	result := append([]collect.Container(nil), items...)
	sort.SliceStable(result, func(i, j int) bool {
		a, b := result[i], result[j]
		switch by {
		case containerSortMemory:
			if a.MemPct != b.MemPct {
				return a.MemPct > b.MemPct
			}
		case containerSortName:
			if a.Name != b.Name {
				return a.Name < b.Name
			}
		default:
			if a.CPUPct != b.CPUPct {
				return a.CPUPct > b.CPUPct
			}
		}
		return a.Name < b.Name
	})
	return result
}

func (s *containerScreen) apply(items []collect.Container, err error) {
	if err == nil {
		s.items = append([]collect.Container(nil), items...)
	}
	s.err = err
	s.status = diagnosticsResultStatus(err, len(s.items) > 0)
}

func (m Model) renderContainers() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("sshmon · "+m.selectedName()+" · Docker") + "\n\n")
	b.WriteString("ИМЯ             ОБРАЗ                  СТАТУС             CPU     MEM\n")
	for _, c := range sortContainers(m.containers.items, m.containers.sort) {
		b.WriteString(fmt.Sprintf("%-16s %-22s %-18s %6.1f%% %6.1f%%\n", c.Name, c.Image, c.Status, c.CPUPct, c.MemPct))
	}
	b.WriteString("\n" + dimStyle.Render(diagnosticsFooter(m.containers.status, m.containers.err)))
	return b.String()
}
