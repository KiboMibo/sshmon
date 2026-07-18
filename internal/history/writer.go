package history

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/kibomibo/sshmon/internal/config"
)

type writeRequest struct {
	ctx    context.Context
	sample Sample
	result chan error
}

type Service struct {
	store    *Store
	options  Options
	requests chan writeRequest
	mu       sync.RWMutex
	closed   bool
	wg       sync.WaitGroup
}

func NewService(store *Store, options Options) *Service {
	options = options.withDefaults()
	service := &Service{
		store: store, options: options,
		requests: make(chan writeRequest, options.QueueSize),
	}
	service.wg.Add(1)
	go service.runWriter()
	return service
}

func OpenService(cfg config.History) (*Service, error) {
	if !cfg.IsEnabled() {
		return nil, nil
	}
	store, err := Open(cfg.Path)
	if err != nil {
		return nil, err
	}
	return NewService(store, Options{
		RawRetention: cfg.RawRetention, AggregateRetention: cfg.AggregateRetention,
	}), nil
}

func (s *Service) Write(ctx context.Context, sample Sample) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return ErrClosed
	}
	request := writeRequest{ctx: ctx, sample: sample, result: make(chan error, 1)}
	select {
	case s.requests <- request:
		s.mu.RUnlock()
	case <-ctx.Done():
		s.mu.RUnlock()
		return ctx.Err()
	}
	select {
	case err := <-request.result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	close(s.requests)
	s.mu.Unlock()
	s.wg.Wait()
	return s.store.Close()
}

func (s *Service) runWriter() {
	defer s.wg.Done()
	for request := range s.requests {
		request.result <- s.write(request.ctx, request.sample)
	}
}

func (s *Service) write(ctx context.Context, sample Sample) error {
	if !sample.Online {
		sample.CPU = nil
		sample.Memory = nil
		sample.Disk = nil
		sample.NetRX = nil
		sample.NetTX = nil
		sample.Load1 = nil
	}
	issues, err := json.Marshal(sample.Issues)
	if err != nil {
		return fmt.Errorf("marshal history issues: %w", err)
	}
	_, err = s.store.db.ExecContext(ctx, `INSERT INTO metric_samples
		(server_key, sampled_at_ms, online, cpu_pct, mem_pct, disk_pct, net_rx_bps, net_tx_bps, load1, issues_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(server_key, sampled_at_ms) DO UPDATE SET
		online=excluded.online, cpu_pct=excluded.cpu_pct, mem_pct=excluded.mem_pct,
		disk_pct=excluded.disk_pct, net_rx_bps=excluded.net_rx_bps,
		net_tx_bps=excluded.net_tx_bps, load1=excluded.load1, issues_json=excluded.issues_json`,
		sample.ServerKey, sample.At.UnixMilli(), sample.Online, sample.CPU, sample.Memory,
		sample.Disk, sample.NetRX, sample.NetTX, sample.Load1, string(issues))
	if err != nil {
		return fmt.Errorf("write history sample: %w", err)
	}
	return nil
}
