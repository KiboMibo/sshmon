package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/kibomibo/sshmon/internal/collect"
)

func TestCountStatesSplitsNormalWarnAndOffline(t *testing.T) {
	// Given three servers where one is offline and one has an issue.
	servers := []collect.Metrics{{Name: "a", Online: true}, {Name: "b", Online: true}, {Name: "c"}}
	issues := []collect.Issue{{Server: "b", Severity: "warn"}}
	// When the fleet states are counted.
	counts := countStates(servers, issues)
	// Then each server lands in exactly one bucket.
	if counts.ok != 1 || counts.warn != 1 || counts.down != 1 || counts.total() != 3 {
		t.Fatalf("counts = %+v", counts)
	}
}

func TestFleetTilesMarkActiveGroupAndAppendAll(t *testing.T) {
	// Given servers from two groups with the second group selected.
	servers := []collect.Metrics{{Name: "a", Group: "prod", Online: true}, {Name: "b", Group: "data"}}
	// When the group tiles are built.
	tiles := fleetTiles(servers, nil, "data")
	// Then groups keep config order, every tile is boxed and a totals tile closes the row.
	if len(tiles) != 3 || !strings.Contains(tiles[0], "prod 1") || !strings.Contains(tiles[1], "data 1") {
		t.Fatalf("tiles = %q", tiles)
	}
	if !strings.Contains(tiles[2], "всё 2") || !strings.Contains(tiles[2], "╭") {
		t.Fatalf("totals tile = %q", tiles[2])
	}
	// And the selected group is styled apart from the same tile when inactive.
	if fleetTile("data", fleetCounts{down: 1}, true) == fleetTile("data", fleetCounts{down: 1}, false) {
		t.Fatal("active tile is not distinguishable")
	}
}

func TestFormatShortDurationPicksCoarsestUnit(t *testing.T) {
	// Given durations spanning seconds, minutes and hours.
	// When each is formatted for the header hint.
	// Then the coarsest fitting unit is used.
	for _, tc := range []struct {
		in   time.Duration
		want string
	}{{3 * time.Second, "3с"}, {90 * time.Second, "2м"}, {2 * time.Hour, "2ч"}} {
		if got := formatShortDuration(tc.in); got != tc.want {
			t.Fatalf("formatShortDuration(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSpreadPushesRightSegmentToTheEdge(t *testing.T) {
	// Given a header split into a left and a right segment.
	// When it is spread across a fixed width.
	line := spread("left", "right", 20)
	// Then the line fills the width exactly and keeps both segments.
	if len([]rune(line)) != 20 || !strings.HasPrefix(line, "left") || !strings.HasSuffix(line, "right") {
		t.Fatalf("spread = %q", line)
	}
}
