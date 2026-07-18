package tui

import "testing"

func TestSparklineHasRequestedWidthAndSignal(t *testing.T) {
	// Given a short metric history.
	values := []float64{0, 20, 40, 60, 80, 100}
	// When it is rendered into four cells.
	line := sparkline(values, 4)
	// Then output has the requested rune width and is not flat.
	if len([]rune(line)) != 4 || line == "    " || line == "▁▁▁▁" {
		t.Fatalf("sparkline = %q", line)
	}
}

func TestSparklineHandlesEmptyHistory(t *testing.T) {
	// Given no samples.
	// When a sparkline is requested.
	line := sparkline(nil, 5)
	// Then a stable empty placeholder is returned.
	if line != "─────" {
		t.Fatalf("sparkline = %q", line)
	}
}
