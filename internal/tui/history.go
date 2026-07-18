package tui

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kibomibo/sshmon/internal/history"
)

type historyStatus uint8

const (
	historyIdle historyStatus = iota
	historyLoading
	historyReady
	historyError
)

type historyMetric uint8

const (
	historyMetricCPU historyMetric = iota
	historyMetricMemory
	historyMetricDisk
	historyMetricNetRX
	historyMetricNetTX
	historyMetricLoad1
)

var historyRanges = [...]history.Range{
	history.Range1H,
	history.Range6H,
	history.Range24H,
	history.Range7D,
	history.Range30D,
}

var historyRangeLabels = [...]string{"1h", "6h", "24h", "7d", "30d"}

type historyScreen struct {
	points        []history.Point
	selectedRange int
	metric        historyMetric
	cursor        int
	status        historyStatus
	err           error
	generation    uint64
	cancel        context.CancelFunc
}

type historyResultMsg struct {
	generation uint64
	points     []history.Point
	err        error
}

// WithHistory подключает необязательное локальное хранилище истории к TUI.
func (m Model) WithHistory(service *history.Service) Model {
	m.historyDB = service
	return m
}

func (m *Model) startHistoryQuery() tea.Cmd {
	m.cancelHistoryQuery()
	m.request = max(m.request, m.history.generation)
	m.request++
	m.history.generation = m.request
	m.history.status = historyLoading
	m.history.err = nil
	generation := m.request
	ctx, cancel := context.WithCancel(context.Background())
	m.history.cancel = cancel
	service := m.historyDB
	serverKey, err := m.selectedServerKey()
	historyRange := historyRanges[m.history.selectedRange]
	return func() tea.Msg {
		if err != nil {
			return historyResultMsg{generation: generation, err: err}
		}
		if service == nil {
			return historyResultMsg{generation: generation, err: errors.New("история недоступна")}
		}
		points, queryErr := service.Query(ctx, serverKey, historyRange)
		return historyResultMsg{generation: generation, points: points, err: queryErr}
	}
}

func (m *Model) cancelHistoryQuery() {
	if m.history.cancel != nil {
		m.history.cancel()
		m.history.cancel = nil
	}
}

func (m Model) selectedServerKey() (string, error) {
	if m.config == nil || m.selected < 0 || m.selected >= len(m.config.Servers) {
		return "", errors.New("сервер не выбран")
	}
	return history.ServerKey(m.config.Servers[m.selected]), nil
}

func (s *historyScreen) apply(points []history.Point, err error) {
	s.err = err
	if err != nil {
		s.status = historyError
		return
	}
	s.points = points
	s.status = historyReady
	if len(points) == 0 {
		s.cursor = 0
	} else if s.cursor >= len(points) {
		s.cursor = len(points) - 1
	}
}

func (m *Model) handleHistoryKey(value string) (tea.Cmd, bool) {
	switch value {
	case "1", "2", "3", "4", "5":
		m.history.selectedRange = int(value[0] - '1')
		return m.startHistoryQuery(), true
	case "r":
		return m.startHistoryQuery(), true
	case "j", "down":
		m.history.metric = historyMetric((int(m.history.metric) + 1) % 6)
		return nil, true
	case "k", "up":
		m.history.metric = historyMetric((int(m.history.metric) + 5) % 6)
		return nil, true
	case "h", "left":
		m.history.cursor = max(0, m.history.cursor-1)
		return nil, true
	case "l", "right":
		m.history.cursor = min(max(0, len(m.history.points)-1), m.history.cursor+1)
		return nil, true
	case "esc":
		return nil, false
	default:
		return nil, false
	}
}

func historyMetricValues(points []history.Point, metric historyMetric) []*float64 {
	values := make([]*float64, len(points))
	for index := range points {
		switch metric {
		case historyMetricCPU:
			values[index] = points[index].CPU
		case historyMetricMemory:
			values[index] = points[index].Memory
		case historyMetricDisk:
			values[index] = points[index].Disk
		case historyMetricNetRX:
			values[index] = points[index].NetRX
		case historyMetricNetTX:
			values[index] = points[index].NetTX
		case historyMetricLoad1:
			values[index] = points[index].Load1
		}
	}
	return values
}

func renderHistoryGraph(values []*float64, width int) string {
	return historySparkline(values, width)
}

func historyCursorLabel(points []history.Point, metric historyMetric, cursor int) string {
	if cursor < 0 || cursor >= len(points) {
		return "нет данных"
	}
	value := historyMetricValues(points[cursor:cursor+1], metric)[0]
	if value == nil {
		return points[cursor].At.UTC().Format("15:04:05") + " · offline"
	}
	return fmt.Sprintf("%s · %.2f", points[cursor].At.UTC().Format("15:04:05"), *value)
}

func (m Model) renderHistory() string {
	var out strings.Builder
	fmt.Fprintf(&out, "%s\n", titleStyle.Render("sshmon · "+m.selectedName()+" · История"))
	if m.history.status == historyError {
		fmt.Fprintf(&out, "ошибка истории: %v\n", m.history.err)
	}
	values := historyMetricValues(m.history.points, m.history.metric)
	graphWidth := max(8, min(80, m.layout.width-2))
	fmt.Fprintf(&out, "%s %s\n", historyMetricLabel(m.history.metric), renderHistoryGraph(values, graphWidth))
	if low, high, ok := historyMinMax(values); ok {
		fmt.Fprintf(&out, "min %.2f · max %.2f\n", low, high)
	}
	fmt.Fprintf(&out, "%s\n", historyCursorLabel(m.history.points, m.history.metric, m.history.cursor))
	fmt.Fprintf(&out, "диапазон %s · 1-5 диапазон · j/k метрика · h/l курсор · r обновить · esc назад", historyRangeLabels[m.history.selectedRange])
	return out.String()
}

func historyMetricLabel(metric historyMetric) string {
	return [...]string{"CPU", "MEM", "DISK", "NET RX", "NET TX", "LOAD1"}[metric]
}

func historyMinMax(values []*float64) (float64, float64, bool) {
	low, high := math.Inf(1), math.Inf(-1)
	for _, value := range values {
		if value != nil {
			low = min(low, *value)
			high = max(high, *value)
		}
	}
	return low, high, !math.IsInf(low, 1)
}
