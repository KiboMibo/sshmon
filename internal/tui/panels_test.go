package tui

import (
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
