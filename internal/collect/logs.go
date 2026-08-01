package collect

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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

// LogStream — поток строк лога, не зависящий от транспорта. Поля повторяют
// sshx.Stream, но тип объявлен здесь: экраны TUI работают с логами через
// collect и не должны знать, что под ними SSH-сессия.
type LogStream struct {
	Lines  <-chan string
	Errors <-chan error
	Close  func() error
}

func (c *Collector) StreamLogs(ctx context.Context, request LogRequest) (LogStream, error) {
	client, err := c.clientFor(request.Server)
	if err != nil {
		return LogStream{}, err
	}
	command, err := c.logCommand(ctx, request)
	if err != nil {
		return LogStream{}, err
	}
	stream, err := client.StreamContext(ctx, command)
	if err != nil {
		return LogStream{}, err
	}
	return LogStream{Lines: stream.Lines, Errors: stream.Errors, Close: stream.Close}, nil
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
		id, err := c.containerID(ctx, request.Server, request.Source.Name)
		if err != nil {
			return "", err
		}
		return "docker logs -f --tail 200 " + id, nil
	default:
		return "", ErrUnsupported
	}
}

func (c *Collector) containerID(ctx context.Context, server, name string) (string, error) {
	containers, err := c.Containers(ctx, server)
	if err != nil {
		return "", err
	}
	for _, container := range containers {
		if (container.ID == name || container.Name == name) && safeLogName.MatchString(container.ID) {
			return container.ID, nil
		}
	}
	return "", fmt.Errorf("неизвестный контейнер %q", name)
}

// snapshotLineCap — жёсткий предел статичного хвоста лога для дашборда.
const snapshotLineCap = 50

func (c *Collector) LogSnapshot(ctx context.Context, request LogRequest, lines int) ([]string, error) {
	client, err := c.clientFor(request.Server)
	if err != nil {
		return nil, err
	}
	command, err := c.logSnapshotCommand(ctx, request, lines)
	if err != nil {
		return nil, err
	}
	raw, err := client.RunContext(ctx, command)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimRight(raw, "\n")
	if trimmed == "" {
		return nil, nil
	}
	// Снимок минует LogBuffer (его показывает плитка логов на экране сервера),
	// поэтому чистим здесь.
	return sanitizeFields(strings.Split(trimmed, "\n")), nil
}

func (c *Collector) logSnapshotCommand(ctx context.Context, request LogRequest, lines int) (string, error) {
	if lines <= 0 || lines > snapshotLineCap {
		lines = snapshotLineCap
	}
	n := strconv.Itoa(lines)
	switch request.Source.Kind {
	case LogSystem:
		return "journalctl -n " + n + " --no-pager 2>/dev/null || tail -n " + n + " /var/log/syslog 2>/dev/null || tail -n " + n + " /var/log/messages 2>/dev/null || logread", nil
	case LogJournal:
		if !safeLogName.MatchString(request.Source.Name) {
			return "", errors.New("недопустимое имя journal unit")
		}
		return "journalctl -n " + n + " --no-pager -u " + request.Source.Name, nil
	case LogContainer:
		id, err := c.containerID(ctx, request.Server, request.Source.Name)
		if err != nil {
			return "", err
		}
		return "docker logs --tail " + n + " " + id, nil
	default:
		return "", ErrUnsupported
	}
}

type LogBuffer struct {
	mu       sync.RWMutex
	maxLines int
	lines    []string
	start    int // индекс первой актуальной строки в lines
	paused   bool
	frozen   []string
	filter   string
	// version растёт на каждое изменение видимого содержимого. Экран логов
	// спрашивает Visible() по нескольку раз за кадр, а буфер держит до 10 000
	// строк: по версии он понимает, что пересчитывать нечего.
	version uint64
}

func NewLogBuffer(maxLines int) *LogBuffer {
	if maxLines <= 0 {
		maxLines = 10_000
	}
	return &LogBuffer{maxLines: maxLines}
}

// Append укладывает строку в буфер. Здесь же — граница доверия: строка пришла с
// удалённого хоста, и дальше её увидят и экран логов, и ящик логов флота, и
// буфер обмена по «y». Чистим один раз на входе, а не в каждом из них.
func (b *LogBuffer) Append(line string) {
	line = SanitizeLine(line)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.version++
	b.lines = append(b.lines, line)
	if len(b.lines)-b.start > b.maxLines {
		b.start++
	}
	// Сдвиг пачкой, а не на каждой строке: при живом `journalctl -f` копирование
	// всего буфера на каждую строку было O(n) на строку. Один сдвиг на maxLines
	// вставок даёт амортизированное O(1); цена — до 2×maxLines заголовков строк.
	if b.start >= b.maxLines {
		kept := copy(b.lines, b.lines[b.start:])
		clear(b.lines[kept:]) // хвост держал бы ссылки на уже выброшенные строки
		b.lines = b.lines[:kept]
		b.start = 0
	}
}

// window — актуальные строки буфера в хронологическом порядке (только под mu).
func (b *LogBuffer) window() []string {
	if b.paused {
		return b.frozen
	}
	return b.lines[b.start:]
}

// Reset очищает накопленные строки. Буфер переиспользуют при смене хоста или
// источника, и строки прежнего потока не должны оставаться под новым заголовком.
// Пауза и фильтр — состояние экрана, а не потока, поэтому сохраняются.
func (b *LogBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.version++
	b.lines = nil
	b.start = 0
	b.frozen = nil
}

func (b *LogBuffer) SetPaused(paused bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.version++
	if paused && !b.paused {
		b.frozen = append([]string(nil), b.lines[b.start:]...)
	}
	if !paused {
		b.frozen = nil
	}
	b.paused = paused
}

func (b *LogBuffer) SetFilter(filter string) {
	b.mu.Lock()
	b.version++
	b.filter = strings.ToLower(filter)
	b.mu.Unlock()
}

// Version — счётчик изменений видимого содержимого: строк, фильтра и паузы.
// Одинаковое значение гарантирует, что Visible() вернёт то же самое.
func (b *LogBuffer) Version() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.version
}

func (b *LogBuffer) Total() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.window())
}

func (b *LogBuffer) Visible() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	lines := b.window()
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
