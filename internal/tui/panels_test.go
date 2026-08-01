package tui

import (
	"math"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestGaugeClampsPercentageAndKeepsExactWidth(t *testing.T) {
	// Given: percentages outside the valid range.
	for _, tc := range []struct {
		value float64
		want  string
	}{
		{value: -10, want: "░░░░░░░░░░"},
		{value: 50, want: "█████░░░░░"},
		{value: 150, want: "██████████"},
	} {
		// When: a ten-cell gauge is rendered.
		got := gauge(tc.value, 10)

		// Then: it is clamped and occupies exactly ten terminal cells.
		if got != tc.want || lipgloss.Width(got) != 10 {
			t.Fatalf("gauge(%v, 10) = %q (width %d), want %q", tc.value, got, lipgloss.Width(got), tc.want)
		}
	}
}

func TestHistorySparklinePreservesGapsAndWidth(t *testing.T) {
	// Given: a short metric history with one offline gap.
	a, b, c := 10.0, 90.0, 40.0
	values := []*float64{&a, nil, &b, &c}

	// When: it is rendered to six terminal cells.
	got := historySparkline(values, 6)

	// Then: the gap is visible and the requested width is exact.
	if !strings.Contains(got, " ") {
		t.Fatalf("sparkline did not preserve offline gap: %q", got)
	}
	if lipgloss.Width(got) != 6 {
		t.Fatalf("sparkline width = %d, want 6: %q", lipgloss.Width(got), got)
	}
}

func TestHistorySparklineUsesPlaceholderForEmptySeries(t *testing.T) {
	// Given: no historical samples.
	// When: a five-cell sparkline is rendered.
	got := historySparkline(nil, 5)

	// Then: a stable placeholder occupies the requested width.
	if got != "─────" {
		t.Fatalf("empty history sparkline = %q, want %q", got, "─────")
	}
}

func TestHistorySparklineNormalisesNonPercentSeries(t *testing.T) {
	// Дано: байты/с — значения далеко за пределами процентной шкалы.
	low, mid, high := 8000.0, 12000.0, 16000.0
	values := []*float64{&low, &mid, &high}

	// Когда: серия рисуется в три ячейки.
	got := historySparkline(values, 3)

	// Тогда: виден тренд, а не сплошная заливка «█».
	glyphs := []rune(got)
	if len(glyphs) != 3 {
		t.Fatalf("sparkline = %q, want 3 glyphs", got)
	}
	if glyphs[0] == glyphs[2] {
		t.Fatalf("sparkline is flat for a rising byte series: %q", got)
	}
	if glyphs[0] != '▁' || glyphs[2] != '█' {
		t.Fatalf("sparkline = %q, want min glyph first and max glyph last", got)
	}
}

func TestHistorySparklineDrawsFlatSeriesAtBaseline(t *testing.T) {
	// Дано: серия без разброса.
	value := 42.0
	values := []*float64{&value, &value, &value}

	// Когда: она рисуется.
	got := historySparkline(values, 3)

	// Тогда: ровная линия по нижнему глифу — у серии нет тренда.
	if got != "▁▁▁" {
		t.Fatalf("flat sparkline = %q, want %q", got, "▁▁▁")
	}
}

func TestFitLineDoesNotBreakAnsiSequences(t *testing.T) {
	// Дано: строка, у которой стиль открыт в начале, а сброс — в самом конце.
	// Escape-коды заданы литералами: под go test lipgloss отдаёт Ascii-профиль
	// и Render() не вставил бы последовательностей вовсе.
	styled := "\x1b[2m" + strings.Repeat("длинный футер ", 8) + "\x1b[0m"

	// Когда: она обрезается до 20 ячеек.
	got := fitLine(styled, 20)

	// Тогда: ширина соблюдена, escape-коды целы, стиль закрыт.
	if lipgloss.Width(got) > 20 {
		t.Fatalf("fitLine width = %d, want <= 20: %q", lipgloss.Width(got), got)
	}
	assertCompleteEscapes(t, got)
	if !strings.HasSuffix(got, "\x1b[0m") {
		t.Fatalf("fitLine left the style open: %q", got)
	}
}

func TestFitLineCutsInsideHighlightWithoutSplittingEscape(t *testing.T) {
	// Дано: подсветка совпадения начинается ровно у границы обрезки.
	line := strings.Repeat("a", 18) + "\x1b[33mсовпадение\x1b[0m" + "хвост"

	// Когда: строка обрезается до 20 ячеек.
	got := fitLine(line, 20)

	// Тогда: обрезка не оставила «голый» \x1b[ и закрыла стиль.
	if lipgloss.Width(got) > 20 {
		t.Fatalf("fitLine width = %d, want <= 20: %q", lipgloss.Width(got), got)
	}
	assertCompleteEscapes(t, got)
	if !strings.HasSuffix(got, "\x1b[0m") {
		t.Fatalf("fitLine left the highlight style open: %q", got)
	}
}

// assertCompleteEscapes проверяет, что каждая ESC-последовательность в строке
// дописана до финального байта — обрыв внутри неё съедает следующие байты кадра.
func assertCompleteEscapes(t *testing.T, value string) {
	t.Helper()
	for index := 0; index < len(value); index++ {
		if value[index] != 0x1b {
			continue
		}
		rest := value[index+1:]
		if !strings.HasPrefix(rest, "[") {
			t.Fatalf("escape-последовательность обрезана в %q", value)
		}
		if strings.IndexFunc(rest[1:], func(r rune) bool { return r >= 0x40 && r <= 0x7e }) < 0 {
			t.Fatalf("незавершённая CSI в %q", value)
		}
	}
}

func TestMetricRowAlignsColumnsAtBoundaryWidths(t *testing.T) {
	// Дано: две строки сетки с разными метками и деталями.
	value := 10.0
	series := []*float64{&value}
	for _, width := range []int{60, 100} {
		cpu := metricRow("CPU", func(w int) string { return historySparkline(series, w) }, 5, "ДЕТАЛИcpu", width)
		mem := metricRow("ПАМЯТЬ", func(w int) string { return gauge(50, w) }, 50, "ДЕТАЛИmem", width)

		// Когда/тогда: обе укладываются в ширину и делят одну левую границу деталей.
		for _, row := range []string{cpu, mem} {
			if lipgloss.Width(row) > width {
				t.Fatalf("metricRow width = %d, want <= %d: %q", lipgloss.Width(row), width, row)
			}
		}
		if got, want := runeIndexOf(cpu, "ДЕТАЛИcpu"), runeIndexOf(mem, "ДЕТАЛИmem"); got != want || got < 0 {
			t.Fatalf("details columns differ at width %d: %d vs %d (%q / %q)", width, got, want, cpu, mem)
		}
	}
}

func TestMetricRowRendersPlaceholderAndFullPercent(t *testing.T) {
	// Дано: тренда у строки нет вовсе и загрузка выше 99%.
	row := metricRow("ДИСК", nil, 100, "", 60)

	// Тогда: колонка тренда пуста (мёртвой черты нет), а процент не расползся.
	if strings.ContainsAny(row, "─█░▁") {
		t.Fatalf("колонка тренда без тренда должна быть пустой: %q", row)
	}
	if !strings.Contains(row, "100%") {
		t.Fatalf("metricRow lost the percent column: %q", row)
	}
	if lipgloss.Width(row) > 60 {
		t.Fatalf("metricRow width = %d, want <= 60: %q", lipgloss.Width(row), row)
	}
	// И: NaN (пустой /proc, раздел нулевого размера) не печатается: «NaN%» на
	// ячейку шире «100%» сдвинул бы всю сетку.
	nan := metricRow("ДИСК", nil, math.NaN(), "", 60)
	if strings.Contains(nan, "NaN") || lipgloss.Width(nan) != lipgloss.Width(row) {
		t.Fatalf("NaN просочился в колонку процента: %q", nan)
	}
}

func TestMetricRowLeavesPercentColumnEmptyWithoutScale(t *testing.T) {
	// Дано: у NET процента нет (байты/с), у CPU — есть.
	value := 10.0
	series := []*float64{&value}
	net := metricRow("NET", nil, metricNoPercent, "rx 1.2M/s", 80)
	cpu := metricRow("CPU", func(w int) string { return historySparkline(series, w) }, 16, "load 1.19", 80)

	// Тогда: колонка процента у NET пустая, но ширину держит — детали обеих
	// строк начинаются в одной колонке.
	if strings.Contains(net, "%") {
		t.Fatalf("у строки без шкалы не должно быть процента: %q", net)
	}
	if got, want := runeIndexOf(net, "rx"), runeIndexOf(cpu, "load"); got != want || got < 0 {
		t.Fatalf("детали разъехались: %d vs %d (%q / %q)", got, want, net, cpu)
	}
}

func TestMetricRowKeepsStyledDetailsIntact(t *testing.T) {
	// Дано: детали приходят цветными, а ширины на них не хватает.
	details := "\x1b[2mload 1.19 0.98 0.71\x1b[0m"

	// Когда: строка сетки рисуется в узкий терминал.
	row := metricRow("CPU", nil, 16, details, 60)

	// Тогда: ширина соблюдена, а ANSI-последовательности целы.
	if lipgloss.Width(row) > 60 {
		t.Fatalf("metricRow width = %d, want <= 60: %q", lipgloss.Width(row), row)
	}
	assertCompleteEscapes(t, row)
}

// runeIndexOf возвращает позицию подстроки в ячейках, а не в байтах:
// глифы спарклайна многобайтовые, байтовый индекс о выравнивании не говорит.
func runeIndexOf(value, substring string) int {
	index := strings.Index(value, substring)
	if index < 0 {
		return -1
	}
	return len([]rune(value[:index]))
}
