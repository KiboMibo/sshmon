package collect

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/kibomibo/sshmon/internal/sshx"
)

type LogSourceKind string

const (
	LogSystem    LogSourceKind = "system"
	LogJournal   LogSourceKind = "journal"
	LogContainer LogSourceKind = "container"
)

type LogSource struct {
	Kind LogSourceKind
	Name string
}

type LogRequest struct {
	ID     uint64
	Server string
	Source LogSource
}

var logRequestSequence atomic.Uint64

func nextLogRequestID() uint64 { return logRequestSequence.Add(1) }

func NewLogRequest(server string, source LogSource) LogRequest {
	return LogRequest{ID: nextLogRequestID(), Server: server, Source: source}
}

func (c *Collector) StreamLogs(ctx context.Context, request LogRequest) (sshx.Stream, error) {
	client, err := c.clientFor(request.Server)
	if err != nil {
		return sshx.Stream{}, err
	}
	command, err := c.logCommand(ctx, request)
	if err != nil {
		return sshx.Stream{}, err
	}
	return client.StreamContext(ctx, command)
}

var safeLogName = regexp.MustCompile(`^[A-Za-z0-9_.@:-]+$`)

func (c *Collector) logCommand(ctx context.Context, request LogRequest) (string, error) {
	switch request.Source.Kind {
	case LogSystem:
		return "journalctl -f -n 200 --no-pager 2>/dev/null || tail -F /var/log/syslog 2>/dev/null || tail -F /var/log/messages 2>/dev/null || logread -f", nil
	case LogJournal:
		if !safeLogName.MatchString(request.Source.Name) {
			return "", errors.New("недопустимое имя journal unit")
		}
		return "journalctl -f -n 200 --no-pager -u " + request.Source.Name, nil
	case LogContainer:
		containers, err := c.Containers(ctx, request.Server)
		if err != nil {
			return "", err
		}
		for _, container := range containers {
			if (container.ID == request.Source.Name || container.Name == request.Source.Name) && safeLogName.MatchString(container.ID) {
				return "docker logs -f --tail 200 " + container.ID, nil
			}
		}
		return "", fmt.Errorf("неизвестный контейнер %q", request.Source.Name)
	default:
		return "", ErrUnsupported
	}
}

type LogBuffer struct {
	mu       sync.RWMutex
	maxLines int
	lines    []string
	paused   bool
	frozen   []string
	filter   string
}

func NewLogBuffer(maxLines int) *LogBuffer {
	if maxLines <= 0 {
		maxLines = 10_000
	}
	return &LogBuffer{maxLines: maxLines}
}

func (b *LogBuffer) Append(line string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lines = append(b.lines, line)
	if excess := len(b.lines) - b.maxLines; excess > 0 {
		b.lines = append([]string(nil), b.lines[excess:]...)
	}
}

func (b *LogBuffer) SetPaused(paused bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if paused && !b.paused {
		b.frozen = append([]string(nil), b.lines...)
	}
	if !paused {
		b.frozen = nil
	}
	b.paused = paused
}

func (b *LogBuffer) SetFilter(filter string) {
	b.mu.Lock()
	b.filter = strings.ToLower(filter)
	b.mu.Unlock()
}

func (b *LogBuffer) Visible() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	lines := b.lines
	if b.paused {
		lines = b.frozen
	}
	if b.filter == "" {
		return append([]string(nil), lines...)
	}
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), b.filter) {
			result = append(result, line)
		}
	}
	return result
}
