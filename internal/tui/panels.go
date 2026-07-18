package tui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func gauge(value float64, width int) string {
	if width < 1 {
		return ""
	}
	value = max(0, min(100, value))
	filled := int(math.Round(value * float64(width) / 100))
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func historySparkline(values []*float64, width int) string {
	if width < 1 {
		return ""
	}
	if len(values) == 0 {
		return strings.Repeat("─", width)
	}
	glyphs := []rune("▁▂▃▄▅▆▇█")
	var out strings.Builder
	for column := range width {
		index := 0
		if width > 1 && len(values) > 1 {
			index = int(math.Round(float64(column) * float64(len(values)-1) / float64(width-1)))
		}
		value := values[index]
		if value == nil {
			out.WriteRune(' ')
			continue
		}
		clamped := max(0, min(100, *value))
		out.WriteRune(glyphs[int(math.Round(clamped*float64(len(glyphs)-1)/100))])
	}
	return out.String()
}

func percentLine(label string, value float64, width int) string {
	barWidth := max(6, min(20, width-len(label)-8))
	return fmt.Sprintf("%-7s %s %3.0f%%", label, gauge(value, barWidth), value)
}

func byteValue(value float64) string {
	units := []string{"B", "K", "M", "G", "T"}
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	return fmt.Sprintf("%.1f%s", value, units[unit])
}

func fitLine(value string, width int) string {
	if width < 1 || lipgloss.Width(value) <= width {
		return value
	}
	runes := []rune(value)
	for len(runes) > 0 && lipgloss.Width(string(runes)+"…") > width {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}
