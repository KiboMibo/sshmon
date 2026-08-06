package collect

import (
	"context"
	"fmt"

	"github.com/kibomibo/sshmon/internal/sshx"
)

const (
	processesCommand = "command -v ps >/dev/null 2>&1 || { echo " + unsupportedMarker + "; exit 0; }; ps -eo pid=,pcpu=,pmem=,args= 2>/dev/null || ps"
	portsCommand     = "if command -v ss >/dev/null 2>&1; then ss -tulpn 2>/dev/null; elif command -v netstat >/dev/null 2>&1; then netstat -tulpn 2>/dev/null; else echo " + unsupportedMarker + "; fi"
	// stderr слит со stdout намеренно: причину отказа («permission denied»,
	// «Cannot connect to the Docker daemon») docker пишет только туда, а
	// ssh-сессия stderr не забирает — без слияния до экрана доезжает лишь
	// «Process exited with status 1». Разбор отличает такой вывод от списка.
	dockerListCommand  = "command -v docker >/dev/null 2>&1 || { echo " + unsupportedMarker + "; exit 0; }; docker ps --format '{{.ID}}\\t{{.Names}}\\t{{.Image}}\\t{{.Status}}\\t{{.Ports}}\\t{{.RunningFor}}' 2>&1"
	dockerStatsCommand = "command -v docker >/dev/null 2>&1 || { echo " + unsupportedMarker + "; exit 0; }; docker stats --no-stream --format '{{.ID}}\\t{{.CPUPerc}}\\t{{.MemPerc}}\\t{{.MemUsage}}'"
)

func (c *Collector) Processes(ctx context.Context, server string) ([]Process, error) {
	client, err := c.runnerFor(server)
	if err != nil {
		return nil, err
	}
	raw, err := client.RunContext(ctx, processesCommand)
	if err != nil {
		return nil, err
	}
	return ParseProcesses(raw)
}

func (c *Collector) Containers(ctx context.Context, server string) ([]Container, error) {
	client, err := c.runnerFor(server)
	if err != nil {
		return nil, err
	}
	list, err := client.RunContext(ctx, dockerListCommand)
	if err != nil {
		return nil, err
	}
	if hasUnsupportedMarker(list) {
		return nil, ErrUnsupported
	}
	stats, err := client.RunContext(ctx, dockerStatsCommand)
	if err != nil {
		return nil, err
	}
	return ParseContainers(list, stats)
}

func (c *Collector) Ports(ctx context.Context, server string) ([]Port, error) {
	client, err := c.runnerFor(server)
	if err != nil {
		return nil, err
	}
	raw, err := client.RunContext(ctx, portsCommand)
	if err != nil {
		return nil, err
	}
	return ParsePorts(raw)
}

// runnerFor — тот же клиент сервера, но за интерфейсом pollRunner. Диагностике
// хватает RunContext, а через интерфейс её можно опросить без живого SSH;
// clientFor остаётся для StreamLogs, которому нужен StreamContext.
func (c *Collector) runnerFor(server string) (pollRunner, error) {
	state, err := c.stateByName(server)
	if err != nil {
		return nil, err
	}
	return state.runner, nil
}

func (c *Collector) clientFor(server string) (*sshx.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, state := range c.states {
		if state.cfg.Name == server {
			return state.cli, nil
		}
	}
	return nil, fmt.Errorf("неизвестный сервер %q", server)
}
