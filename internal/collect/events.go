package collect

import (
	"fmt"
	"sync"

	"github.com/kibomibo/sshmon/internal/history"
)

type Event struct {
	Snapshot Snapshot
}

// Subscribe возвращает поток последних снимков и идемпотентную функцию отписки.
func (c *Collector) Subscribe(buffer int) (<-chan Event, func()) {
	if buffer < 1 {
		buffer = 1
	}
	ch := make(chan Event, buffer)
	c.mu.Lock()
	if c.subscribers == nil {
		c.subscribers = make(map[uint64]chan Event)
	}
	id := c.nextSubID
	c.nextSubID++
	c.subscribers[id] = ch
	c.mu.Unlock()

	var once sync.Once
	return ch, func() {
		once.Do(func() {
			c.mu.Lock()
			delete(c.subscribers, id)
			close(ch)
			c.mu.Unlock()
		})
	}
}

func (c *Collector) publish(event Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, ch := range c.subscribers {
		select {
		case ch <- event:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- event:
			default:
			}
		}
	}
}

func (c *Collector) historySamples(snapshot Snapshot) []history.Sample {
	issues := make(map[string][]string)
	for _, issue := range snapshot.Issues {
		issues[issue.Server] = append(issues[issue.Server], fmt.Sprintf("[%s] %s", issue.Severity, issue.Msg))
	}

	samples := make([]history.Sample, 0, len(snapshot.Servers))
	for i, metrics := range snapshot.Servers {
		if i >= len(c.cfg.Servers) {
			break
		}
		sample := history.Sample{
			ServerKey: history.ServerKey(c.cfg.Servers[i]),
			At:        snapshot.Time,
			Online:    metrics.Online,
			Issues:    issues[metrics.Name],
		}
		if metrics.Online {
			sample.CPU = floatPtr(metrics.CPUPct)
			sample.Memory = floatPtr(metrics.MemPct)
			sample.Load1 = floatPtr(metrics.Load1)
			if len(metrics.Disks) > 0 {
				disk := 0.0
				for _, usage := range metrics.Disks {
					if usage.UsedPct > disk {
						disk = usage.UsedPct
					}
				}
				sample.Disk = floatPtr(disk)
			}
			if len(metrics.Net) > 0 {
				rx, tx := 0.0, 0.0
				for _, rate := range metrics.Net {
					rx += rate.RxBps
					tx += rate.TxBps
				}
				sample.NetRX = floatPtr(rx)
				sample.NetTX = floatPtr(tx)
			}
		}
		samples = append(samples, sample)
	}
	return samples
}

func floatPtr(value float64) *float64 {
	return &value
}
