package history

import (
	"context"
	"fmt"
	"time"
)

func (s *Service) Maintain(ctx context.Context, now time.Time) error {
	tx, err := s.store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin history maintenance: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	completedBefore := now.Truncate(time.Minute).UnixMilli()
	if _, err := tx.ExecContext(ctx, `INSERT INTO metric_rollups_minute
		(server_key, bucket_at_ms, online_count, sample_count,
		 cpu_min, cpu_max, cpu_avg, mem_min, mem_max, mem_avg,
		 disk_min, disk_max, disk_avg, net_rx_avg, net_tx_avg, load1_avg)
		SELECT server_key, (sampled_at_ms / 60000) * 60000,
		 SUM(CASE WHEN online THEN 1 ELSE 0 END), COUNT(*),
		 MIN(cpu_pct), MAX(cpu_pct), AVG(cpu_pct),
		 MIN(mem_pct), MAX(mem_pct), AVG(mem_pct),
		 MIN(disk_pct), MAX(disk_pct), AVG(disk_pct),
		 AVG(net_rx_bps), AVG(net_tx_bps), AVG(load1)
		FROM metric_samples WHERE sampled_at_ms < ?
		GROUP BY server_key, (sampled_at_ms / 60000) * 60000
		ON CONFLICT(server_key, bucket_at_ms) DO UPDATE SET
		 online_count=excluded.online_count, sample_count=excluded.sample_count,
		 cpu_min=excluded.cpu_min, cpu_max=excluded.cpu_max, cpu_avg=excluded.cpu_avg,
		 mem_min=excluded.mem_min, mem_max=excluded.mem_max, mem_avg=excluded.mem_avg,
		 disk_min=excluded.disk_min, disk_max=excluded.disk_max, disk_avg=excluded.disk_avg,
		 net_rx_avg=excluded.net_rx_avg, net_tx_avg=excluded.net_tx_avg,
		 load1_avg=excluded.load1_avg`, completedBefore); err != nil {
		return fmt.Errorf("roll up history: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM metric_samples WHERE sampled_at_ms < ?`,
		now.Add(-s.options.RawRetention).UnixMilli()); err != nil {
		return fmt.Errorf("delete raw history: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM metric_rollups_minute WHERE bucket_at_ms < ?`,
		now.Add(-s.options.AggregateRetention).UnixMilli()); err != nil {
		return fmt.Errorf("delete rollup history: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit history maintenance: %w", err)
	}
	return nil
}
