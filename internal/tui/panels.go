package tui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// gaugeFramedMin — с этой ширины бар берётся в скобки. Ниже неё рамка съела бы
// треть шкалы, и «мало» стало бы неотличимо от «много».
const gaugeFramedMin = 6

const (
	// gaugeFilled/gaugeEmpty — фактура шкалы. Пустая часть рисуется точечной
	// дорожкой, а не заливкой «░»: три бара подряд (cpu/mem/disk в карточке и в
	// сетке метрик) сливались в один серый прямоугольник, в котором строки уже
	// не различались. Точка занимает середину ячейки и оставляет над и под
	// собой просвет — тот самый «зазор в пару пикселей», которого в терминале
	// нет. Сплошная «─» тут не годится: ею historySparkline рисует мёртвый ряд,
	// и два разных факта выглядели бы одинаково.
	gaugeFilled = "█"
	gaugeEmpty  = "·"
)

// gauge рисует шкалу ровно в width ячеек. Скобки по краям — не украшение:
// три бара подряд (cpu/mem/disk в карточке хоста) без них сливались в один
// прямоугольник. Рамка входит в width, поэтому колонка тренда в сетке метрик
// и карточка остаются выровненными.
func gauge(value float64, width int) string {
	if width < 1 {
		return ""
	}
	if width < gaugeFramedMin {
		return gaugeScale(value, width)
	}
	// Скобки тусклые: рамка — служебная разметка, внимание принадлежит заливке.
	return dimStyle.Render("[") + gaugeScale(value, width-2) + dimStyle.Render("]")
}

func gaugeScale(value float64, width int) string {
	// Кламп по числу ячеек, а не по проценту: NaN на входе даёт непредсказуемый
	// int, и strings.Repeat с отрицательным счётчиком уронил бы рендер.
	filled := max(0, min(width, int(math.Round(value*float64(width)/100))))
	// Цвет заливки — по тем же порогам, что у процента рядом: бар и число
	// говорят об одном, и расходиться в оценке они не имеют права. Ниже порогов
	// заливка зелёная, а не бесцветная: одинаковый цвет у всех спокойных баров
	// и есть признак «всё в норме».
	style, alert := percentSeverity(value)
	if !alert {
		style = goodStyle
	}
	return repeatStyled(style, gaugeFilled, filled) + repeatStyled(dimStyle, gaugeEmpty, width-filled)
}

// repeatStyled — count повторов glyph под одним стилем. Пустая часть отдаётся
// пустой строкой: lipgloss на «» всё равно выписал бы пару escape-кодов, а
// fitLine потом считал бы их незакрытым стилем.
func repeatStyled(style lipgloss.Style, glyph string, count int) string {
	if count <= 0 {
		return ""
	}
	return style.Render(strings.Repeat(glyph, count))
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
	metricRowLabelWidth   = 7  // «DISK» плюс отбивка до тренда, как в макете
	metricRowPercentWidth = 4  // «100%»
	metricRowSparkMax     = 30 // шире тренд не читается лучше, остаток отдаём деталям

	// metricNoPercent — договор с metricRow: у строки нет процентной шкалы
	// (так у NET — байты/с в проценты не переводятся), колонка процента
	// остаётся пустой, но её ширина сохраняется, иначе детали разъедутся.
	metricNoPercent = -1.0
)

// metricTrend рисует ячейку колонки тренда в заданную ширину. Ширину считает
// metricRow (она зависит от общей ширины строки), поэтому вызывающий передаёт
// не готовую строку, а способ её нарисовать: заливку gauge, спарклайн истории
// или ничего (nil — колонка остаётся пустой, но ширину держит).
type metricTrend func(width int) string

// metricRow рисует одну строку сетки метрик экрана сервера:
// «МЕТКА  <тренд>  <NN%>  <детали>». Ширины колонок выводятся из общей
// ширины, поэтому строки CPU/MEM/NET/DISK с одинаковым width выравниваются
// друг под другом без расчётов на стороне вызывающего.
func metricRow(label string, trend metricTrend, percent float64, details string, width int) string {
	const gap = "  "
	fixed := metricRowLabelWidth + metricRowPercentWidth + 3*len(gap)
	trendWidth := min(metricRowSparkMax, max(0, width-fixed)/2)
	detailsWidth := width - fixed - trendWidth

	cell := strings.Repeat(" ", trendWidth)
	if trend != nil {
		cell = trend(trendWidth)
	}
	row := padLabel(truncateCells(label, metricRowLabelWidth), metricRowLabelWidth) +
		gap + cell +
		gap + metricPercentCell(percent)
	if details != "" && detailsWidth > 0 {
		// fitLine, а не truncateCells: детали приходят цветными (load, swap, rx),
		// и обрезка по рунам разорвала бы ANSI-последовательность.
		row += gap + fitLine(details, detailsWidth)
	}
	return fitLine(row, width)
}

func metricPercentCell(percent float64) string {
	// NaN приходит из деления на нулевой итог (пустой /proc, диск нулевого
	// размера): «%3.0f» напечатал бы «NaN%» — на ячейку шире, и сетка съехала бы.
	if math.IsNaN(percent) || percent < 0 {
		return strings.Repeat(" ", metricRowPercentWidth)
	}
	value := min(100, percent)
	cell := fmt.Sprintf("%*.0f%%", metricRowPercentWidth-1, value)
	// Число подсвечивается только за порогом: цвет спокойного значения уже
	// несёт заливка бара слева, и красить сюда то же самое — значит потерять
	// выделение как раз там, где оно нужно.
	if style, alert := percentSeverity(value); alert {
		return style.Render(cell)
	}
	return cell
}

// percentSeverity — единственные пороги «внимание / критично» для процентных
// метрик: 75 % и 90 %. Их делят число в сетке метрик и заливка бара, поэтому
// живут они здесь, а не по копии на каждом вызове.
func percentSeverity(value float64) (lipgloss.Style, bool) {
	switch {
	case value >= 90:
		return criticalStyle, true
	case value >= 75:
		return warnStyle, true
	default:
		return lipgloss.Style{}, false
	}
}

func percentLine(label string, value float64, width int) string {
	// Ширина бара от метки не зависит: метка стоит в своей колонке в семь
	// ячеек. От len(label) бар «disk» был на ячейку короче «cpu» и «mem», и
	// три обрамлённых бара подряд заканчивались лесенкой.
	barWidth := max(6, min(20, width-11))
	return fmt.Sprintf("%-7s %s %3.0f%%", label, gauge(value, barWidth), value)
}

// plural выбирает форму русского счётного существительного: 1 ядро, 2 ядра,
// 5 ядер. Отдельная ветка на 11–14 — они склоняются как «много» («11 ядер»,
// «14 ядер»), хотя и оканчиваются на 1–4.
func plural(count int, one, few, many string) string {
	if count < 0 {
		count = -count
	}
	switch {
	case count%100 >= 11 && count%100 <= 14:
		return many
	case count%10 == 1:
		return one
	case count%10 >= 2 && count%10 <= 4:
		return few
	default:
		return many
	}
}

// coresText — «2 ядра» в шапке экрана сервера и в карточке флота: один и тот
// же факт в двух местах должен и склоняться одинаково.
func coresText(count int) string {
	return fmt.Sprintf("%d %s", count, plural(count, "ядро", "ядра", "ядер"))
}

// hostsText — «26 хостов» в шапке флота и в строке области видимости.
func hostsText(count int) string {
	return fmt.Sprintf("%d %s", count, plural(count, "хост", "хоста", "хостов"))
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
// Сохраняются здесь только наши собственные коды: текст с удалённого хоста
// приходит уже чистым (collect.SanitizeLine на входе), а стилизация приложения
// накладывается после — иначе escape-коды чужого лога fitLine бережно донёс бы
// до терминала оператора.
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
