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
	// Кламп по числу ячеек, а не по проценту: NaN на входе даёт непредсказуемый
	// int, и strings.Repeat с отрицательным счётчиком уронил бы рендер.
	filled := max(0, min(width, int(math.Round(value*float64(width)/100))))
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

// historySparkline рисует серию в width ячеек. Значения нормируются по
// фактическому диапазону серии: история хранит не только проценты, но и
// байты/с и load average — прежний кламп в [0,100] превращал 8 КБ/с
// в сплошной «█». nil — пропуск (сервер был offline), рисуется пробелом.
func historySparkline(values []*float64, width int) string {
	if width < 1 {
		return ""
	}
	if len(values) == 0 {
		return strings.Repeat("─", width)
	}
	glyphs := []rune("▁▂▃▄▅▆▇█")
	low, high := seriesRange(values)
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
		// Плоская серия (high == low) рисуется нижним глифом: тренда нет,
		// а линия по центру читалась бы как «половина шкалы».
		level := 0.0
		if high > low {
			level = (*value - low) / (high - low)
		}
		// Кламп индекса, а не значения: страхует от NaN внутри серии.
		out.WriteRune(glyphs[max(0, min(len(glyphs)-1, int(math.Round(level*float64(len(glyphs)-1)))))])
	}
	return out.String()
}

// seriesRange возвращает минимум и максимум по непустым значениям серии.
func seriesRange(values []*float64) (low, high float64) {
	first := true
	for _, value := range values {
		if value == nil {
			continue
		}
		if first || *value < low {
			low = *value
		}
		if first || *value > high {
			high = *value
		}
		first = false
	}
	return low, high
}

const (
	metricRowLabelWidth   = 7  // «DISK» плюс отбивка до спарклайна, как в макете
	metricRowPercentWidth = 4  // «100%»
	metricRowSparkMax     = 30 // шире тренд не читается лучше, остаток отдаём деталям

	// metricNoPercent — договор с metricRow: у строки нет процентной шкалы
	// (так у NET — байты/с в проценты не переводятся), колонка процента
	// остаётся пустой, но её ширина сохраняется, иначе детали разъедутся.
	metricNoPercent = -1.0
)

// metricRow рисует одну строку сетки метрик экрана сервера:
// «МЕТКА  <спарклайн>  <NN%>  <детали>». Ширины колонок выводятся из общей
// ширины, поэтому строки CPU/MEM/NET/DISK с одинаковым width выравниваются
// друг под другом без расчётов на стороне вызывающего.
func metricRow(label string, series []*float64, percent float64, details string, width int) string {
	const gap = "  "
	fixed := metricRowLabelWidth + metricRowPercentWidth + 3*len(gap)
	sparkWidth := min(metricRowSparkMax, max(0, width-fixed)/2)
	detailsWidth := width - fixed - sparkWidth

	row := padLabel(truncateCells(label, metricRowLabelWidth), metricRowLabelWidth) +
		gap + historySparkline(series, sparkWidth) +
		gap + metricPercentCell(percent)
	if details != "" && detailsWidth > 0 {
		// fitLine, а не truncateCells: детали приходят цветными (load, swap, rx),
		// и обрезка по рунам разорвала бы ANSI-последовательность.
		row += gap + fitLine(details, detailsWidth)
	}
	return fitLine(row, width)
}

func metricPercentCell(percent float64) string {
	if percent < 0 {
		return strings.Repeat(" ", metricRowPercentWidth)
	}
	value := min(100, percent)
	cell := fmt.Sprintf("%*.0f%%", metricRowPercentWidth-1, value)
	switch {
	case value >= 90:
		return criticalStyle.Render(cell)
	case value >= 75:
		return warnStyle.Render(cell)
	default:
		return cell
	}
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

func composeScreen(screen, overlay string, layout layoutState) string {
	if layout.height <= 0 {
		if overlay == "" {
			return screen
		}
		return screen + "\n\n" + overlay
	}
	screenLines := strings.Split(strings.TrimSuffix(screen, "\n"), "\n")
	footer := fitLine(screenLines[len(screenLines)-1], layout.width)
	bodyLines := screenLines[:len(screenLines)-1]
	overlayLines := []string(nil)
	if overlay != "" {
		overlayLines = strings.Split(strings.TrimSuffix(overlay, "\n"), "\n")
	}
	available := max(0, layout.height-1)
	if len(overlayLines) > available {
		overlayLines = overlayLines[:available]
	}
	separator := 0
	if len(bodyLines) > 0 && len(overlayLines) > 0 && len(overlayLines) < available {
		separator = 1
	}
	bodyCount := min(len(bodyLines), available-len(overlayLines)-separator)
	lines := append([]string(nil), bodyLines[:bodyCount]...)
	for len(lines)+separator+len(overlayLines) < available {
		lines = append(lines, "")
	}
	if separator > 0 {
		lines = append(lines, "")
	}
	lines = append(lines, overlayLines...)
	return strings.Join(append(lines, footer), "\n")
}

// fitLine обрезает строку до width терминальных ячеек, не разрывая
// ANSI-последовательности: escape-коды копируются целиком и не занимают ячеек.
// Обрезка по []rune ломала кадр — она могла срезать финальный «\x1b[0m» (и
// остаток экрана красился в dim) или остановиться внутри «\x1b[33m» от
// подсветки совпадений, после чего терминал съедал следующие байты.
func fitLine(value string, width int) string {
	if width < 1 || lipgloss.Width(value) <= width {
		return value
	}
	runes := []rune(value)
	limit := width - 1 // одна ячейка под многоточие
	var out strings.Builder
	cells, open := 0, false
	for index := 0; index < len(runes); {
		if runes[index] == 0x1b {
			end := escapeEnd(runes, index)
			sequence := string(runes[index:end])
			out.WriteString(sequence)
			if strings.HasSuffix(sequence, "m") {
				open = sequence != "\x1b[0m" && sequence != "\x1b[m"
			}
			index = end
			continue
		}
		cellWidth := lipgloss.Width(string(runes[index]))
		if cells+cellWidth > limit {
			break
		}
		out.WriteRune(runes[index])
		cells += cellWidth
		index++
	}
	out.WriteString("…")
	if open {
		out.WriteString("\x1b[0m")
	}
	return out.String()
}

// escapeEnd возвращает индекс за концом ANSI-последовательности, начинающейся
// на runes[start] == ESC. Нужен, чтобы обрезка никогда не рвала escape-код.
func escapeEnd(runes []rune, start int) int {
	index := start + 1
	if index >= len(runes) {
		return len(runes)
	}
	switch runes[index] {
	case '[': // CSI: параметры, затем финальный байт 0x40–0x7E
		for index++; index < len(runes); index++ {
			if runes[index] >= 0x40 && runes[index] <= 0x7e {
				return index + 1
			}
		}
	case ']': // OSC: до BEL или ST («ESC \»)
		for index++; index < len(runes); index++ {
			if runes[index] == 0x07 {
				return index + 1
			}
			if runes[index] == 0x1b && index+1 < len(runes) && runes[index+1] == '\\' {
				return index + 2
			}
		}
	default: // двухсимвольная последовательность
		return index + 1
	}
	return len(runes)
}
