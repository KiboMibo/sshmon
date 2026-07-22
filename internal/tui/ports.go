package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kibomibo/sshmon/internal/collect"
)

type portSort uint8

const (
	portSortLocal portSort = iota
	portSortProtocol
	portSortProcess
)

type portScreen struct {
	items []collect.Port
	sort  portSort
	diagnostics
}

func sortPorts(items []collect.Port, by portSort) []collect.Port {
	result := append([]collect.Port(nil), items...)
	sort.SliceStable(result, func(i, j int) bool {
		a, b := result[i], result[j]
		switch by {
		case portSortProtocol:
			if a.Proto != b.Proto {
				return a.Proto < b.Proto
			}
		case portSortProcess:
			if a.Process != b.Process {
				return a.Process < b.Process
			}
		default:
			if a.Local != b.Local {
				return a.Local < b.Local
			}
		}
		if a.Proto != b.Proto {
			return a.Proto < b.Proto
		}
		if a.Local != b.Local {
			return a.Local < b.Local
		}
		if a.Process != b.Process {
			return a.Process < b.Process
		}
		return a.PID < b.PID
	})
	return result
}

func (s *portScreen) apply(items []collect.Port, err error) {
	if err == nil {
		s.items = append([]collect.Port(nil), items...)
	}
	s.finish(err, len(s.items) > 0)
}

var _ screen = portScreen{}

func (s portScreen) view(ctx screenContext) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("sshmon · "+ctx.serverName+" · Порты") + "\n\n")
	b.WriteString("PROTO   LOCAL                         ПРОЦЕСС             PID\n")
	for _, p := range sortPorts(s.items, s.sort) {
		b.WriteString(fmt.Sprintf("%-7s %-29s %-19s %d\n", p.Proto, p.Local, p.Process, p.PID))
	}
	b.WriteString("\n" + dimStyle.Render(diagnosticsFooter(s.status, s.err)))
	return b.String()
}
