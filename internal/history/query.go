package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

func (s *Service) Query(ctx context.Context, serverKey string, historyRange Range) ([]Point, error) {
	cutoff := s.options.Now().Add(-historyRange.Duration).UnixMilli()
	if historyRange.Rollup {
		return s.queryRollups(ctx, serverKey, cutoff)
	}
	return s.queryRaw(ctx, serverKey, cutoff)
}

func (s *Service) queryRaw(ctx context.Context, serverKey string, cutoff int64) ([]Point, error) {
	rows, err := s.store.db.QueryContext(ctx, `SELECT sampled_at_ms, online, cpu_pct, mem_pct,
		disk_pct, net_rx_bps, net_tx_bps, load1, issues_json
		FROM metric_samples WHERE server_key=? AND sampled_at_ms>=? ORDER BY sampled_at_ms`, serverKey, cutoff)
	if err != nil {
		return nil, fmt.Errorf("query raw history: %w", err)
	}
	defer rows.Close()
	var points []Point
	for rows.Next() {
		var at int64
		var online bool
		var cpu, memory, disk, netRX, netTX, load1 sql.NullFloat64
		var issuesJSON string
		if err := rows.Scan(&at, &online, &cpu, &memory, &disk, &netRX, &netTX, &load1, &issuesJSON); err != nil {
			return nil, fmt.Errorf("scan raw history: %w", err)
		}
		point := Point{
			At: time.UnixMilli(at), Online: online,
			CPU: floatPointer(cpu), Memory: floatPointer(memory), Disk: floatPointer(disk),
			NetRX: floatPointer(netRX), NetTX: floatPointer(netTX), Load1: floatPointer(load1),
		}
		if err := json.Unmarshal([]byte(issuesJSON), &point.Issues); err != nil {
			return nil, fmt.Errorf("decode history issues: %w", err)
		}
		points = append(points, point)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate raw history: %w", err)
	}
	return points, nil
}

func (s *Service) queryRollups(ctx context.Context, serverKey string, cutoff int64) ([]Point, error) {
	rows, err := s.store.db.QueryContext(ctx, `SELECT bucket_at_ms, online_count,
		cpu_avg, mem_avg, disk_avg, net_rx_avg, net_tx_avg, load1_avg
		FROM metric_rollups_minute WHERE server_key=? AND bucket_at_ms>=? ORDER BY bucket_at_ms`, serverKey, cutoff)
	if err != nil {
		return nil, fmt.Errorf("query rollup history: %w", err)
	}
	defer rows.Close()
	var points []Point
	for rows.Next() {
		var at int64
		var onlineCount int
		var cpu, memory, disk, netRX, netTX, load1 sql.NullFloat64
		if err := rows.Scan(&at, &onlineCount, &cpu, &memory, &disk, &netRX, &netTX, &load1); err != nil {
			return nil, fmt.Errorf("scan rollup history: %w", err)
		}
		points = append(points, Point{
			At: time.UnixMilli(at), Online: onlineCount > 0,
			CPU: floatPointer(cpu), Memory: floatPointer(memory), Disk: floatPointer(disk),
			NetRX: floatPointer(netRX), NetTX: floatPointer(netTX), Load1: floatPointer(load1),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rollup history: %w", err)
	}
	return points, nil
}

func floatPointer(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	result := value.Float64
	return &result
}
