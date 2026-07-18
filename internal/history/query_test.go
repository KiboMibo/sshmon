package history

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"
)

func TestQueryReturnsOrderedRawPointsWithOfflineGap(t *testing.T) {
	// Given an online sample followed by an offline sample.
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	service := newTestService(t, now)
	cpu := 25.0
	if err := service.Write(context.Background(), Sample{
		ServerKey: "root@web:22", At: now.Add(-2 * time.Minute), Online: true, CPU: &cpu,
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.Write(context.Background(), Sample{
		ServerKey: "root@web:22", At: now.Add(-time.Minute), Online: false,
	}); err != nil {
		t.Fatal(err)
	}

	// When the short-range history is queried.
	points, err := service.Query(context.Background(), "root@web:22", Range1H)
	if err != nil {
		t.Fatal(err)
	}

	// Then points are chronological and the offline point is an explicit numeric gap.
	if len(points) != 2 {
		t.Fatalf("len(points) = %d, want 2", len(points))
	}
	if !points[0].At.Before(points[1].At) || points[0].CPU == nil || *points[0].CPU != 25 {
		t.Fatalf("unexpected online point: %#v", points[0])
	}
	if points[1].Online || points[1].CPU != nil || points[1].Memory != nil {
		t.Fatalf("offline point must be a gap: %#v", points[1])
	}
}

func TestRollupExcludesOfflineMetricsAndLongRangeUsesMinutePoints(t *testing.T) {
	// Given two online values and one offline sample in the same completed minute.
	now := time.Date(2026, 7, 18, 12, 5, 0, 0, time.UTC)
	service := newTestService(t, now)
	for i, value := range []float64{10, 30} {
		value := value
		if err := service.Write(context.Background(), Sample{
			ServerKey: "root@web:22", At: now.Add(-2*time.Minute + time.Duration(i)*10*time.Second),
			Online: true, CPU: &value,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := service.Write(context.Background(), Sample{
		ServerKey: "root@web:22", At: now.Add(-2*time.Minute + 20*time.Second), Online: false,
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.Maintain(context.Background(), now); err != nil {
		t.Fatal(err)
	}

	// When a seven-day range is queried.
	points, err := service.Query(context.Background(), "root@web:22", Range7D)
	if err != nil {
		t.Fatal(err)
	}

	// Then one minute point uses the online average and excludes the offline NULL value.
	if len(points) != 1 || points[0].CPU == nil || math.Abs(*points[0].CPU-20) > 0.001 {
		t.Fatalf("rollup points = %#v, want CPU average 20", points)
	}
	var minimum, maximum, average float64
	var onlineCount, sampleCount int
	if err := service.store.db.QueryRow(`SELECT cpu_min, cpu_max, cpu_avg, online_count, sample_count
		FROM metric_rollups_minute WHERE server_key=?`, "root@web:22").
		Scan(&minimum, &maximum, &average, &onlineCount, &sampleCount); err != nil {
		t.Fatal(err)
	}
	if minimum != 10 || maximum != 30 || average != 20 || onlineCount != 2 || sampleCount != 3 {
		t.Fatalf("rollup = min %.1f max %.1f avg %.1f online %d samples %d", minimum, maximum, average, onlineCount, sampleCount)
	}
}

func newTestService(t *testing.T, now time.Time) *Service {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(store, Options{Now: func() time.Time { return now }})
	t.Cleanup(func() { _ = service.Close() })
	return service
}
