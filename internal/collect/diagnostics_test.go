package collect

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/kibomibo/sshmon/internal/config"
)

// scriptedRunner отвечает разным выводом на разные команды: Containers шлёт две
// (список и stats), и подмена одним ответом на всё скрыла бы порядок вызовов.
type scriptedRunner struct {
	mu       sync.Mutex
	replies  map[string]string
	err      error
	commands []string
}

func (r *scriptedRunner) RunContext(ctx context.Context, command string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands = append(r.commands, command)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if r.err != nil {
		return "", r.err
	}
	return r.replies[command], nil
}

func (r *scriptedRunner) Reset() {}

func (r *scriptedRunner) SetPassphrase([]byte) {}

func (r *scriptedRunner) calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.commands...)
}

func newDiagnosticsCollector(runner pollRunner) *Collector {
	return &Collector{
		cfg:         &config.Config{},
		states:      []*serverState{{cfg: config.Server{Name: "web"}, runner: runner, m: Metrics{Name: "web"}}},
		subscribers: make(map[uint64]chan Event),
	}
}

func TestProcessesParsesRemoteOutput(t *testing.T) {
	t.Parallel()
	// Given a host whose ps returns one usable row among noise.
	runner := &scriptedRunner{replies: map[string]string{
		processesCommand: "  123 12.5 3.2 /usr/bin/nginx -g daemon off;\nmalformed\n",
	}}

	// When the process list is requested.
	got, err := newDiagnosticsCollector(runner).Processes(context.Background(), "web")

	// Then the row is parsed and the agreed command was the one sent.
	if err != nil {
		t.Fatalf("Processes() error = %v", err)
	}
	want := Process{PID: 123, Command: "/usr/bin/nginx -g daemon off;", CPUPct: 12.5, MemPct: 3.2}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("Processes() = %#v, want %#v", got, want)
	}
	if calls := runner.calls(); len(calls) != 1 || calls[0] != processesCommand {
		t.Fatalf("commands = %#v", calls)
	}
}

func TestProcessesReportsUnsupportedAndEmptyOutputSeparately(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		raw     string
		wantErr error
	}{
		// Маркер приходит из самой команды: ps на хосте нет.
		{name: "ps missing", raw: unsupportedMarker + "\n", wantErr: ErrUnsupported},
		{name: "empty output", raw: ""},
		{name: "only header", raw: "PID USER COMMAND\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Given a host answering with the scripted output.
			runner := &scriptedRunner{replies: map[string]string{processesCommand: tt.raw}}

			// When the process list is requested.
			got, err := newDiagnosticsCollector(runner).Processes(context.Background(), "web")

			// Then "utility missing" is an error while "nothing to show" is an empty list.
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Processes() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && len(got) != 0 {
				t.Fatalf("Processes() = %#v, want empty", got)
			}
		})
	}
}

func TestPortsParsesBothSsAndNetstatFlavours(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  string
		want Port
	}{
		{
			name: "ss",
			raw:  "Netid State Recv-Q Send-Q Local:Port Peer\ntcp LISTEN 0 128 0.0.0.0:22 0.0.0.0:* users:((\"sshd\",pid=712,fd=3))\n",
			want: Port{Proto: "tcp", Local: "0.0.0.0:22", Process: "sshd", PID: 712},
		},
		{
			name: "netstat",
			raw:  "tcp 0 0 127.0.0.1:5432 0.0.0.0:* LISTEN 991/postgres\n",
			want: Port{Proto: "tcp", Local: "127.0.0.1:5432", Process: "postgres", PID: 991},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Given a host whose shell fallback picked one of the two utilities.
			runner := &scriptedRunner{replies: map[string]string{portsCommand: tt.raw}}

			// When the listening ports are requested.
			got, err := newDiagnosticsCollector(runner).Ports(context.Background(), "web")

			// Then both output formats end up as the same port record.
			if err != nil {
				t.Fatalf("Ports() error = %v", err)
			}
			if len(got) != 1 || got[0] != tt.want {
				t.Fatalf("Ports() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestPortsReportsUnsupportedWhenNeitherUtilityExists(t *testing.T) {
	t.Parallel()
	// Given a host without ss and without netstat.
	runner := &scriptedRunner{replies: map[string]string{portsCommand: unsupportedMarker + "\n"}}

	// When the listening ports are requested.
	_, err := newDiagnosticsCollector(runner).Ports(context.Background(), "web")

	// Then the absence of the utility is reported as such, not as an empty list.
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Ports() error = %v, want ErrUnsupported", err)
	}
}

func TestPortsOnQuietHostReturnsEmptyList(t *testing.T) {
	t.Parallel()
	// Given a host with no listening sockets.
	runner := &scriptedRunner{replies: map[string]string{portsCommand: "\n"}}

	// When the listening ports are requested.
	got, err := newDiagnosticsCollector(runner).Ports(context.Background(), "web")

	// Then an empty list is not an error.
	if err != nil || len(got) != 0 {
		t.Fatalf("Ports() = %#v, %v; want empty list without error", got, err)
	}
}

func TestContainersCombinesListAndStatsCommands(t *testing.T) {
	t.Parallel()
	// Given a host with docker answering both the list and the one-shot stats.
	runner := &scriptedRunner{replies: map[string]string{
		dockerListCommand:  "abc123\tweb\tnginx:latest\tUp 2 hours\t0.0.0.0:8080->80/tcp\t2 hours ago\n",
		dockerStatsCommand: "abc123\t2.50%\t12.75%\t64MiB / 512MiB\n",
	}}

	// When the container list is requested.
	got, err := newDiagnosticsCollector(runner).Containers(context.Background(), "web")

	// Then both outputs are merged by container ID.
	if err != nil {
		t.Fatalf("Containers() error = %v", err)
	}
	want := Container{
		ID: "abc123", Name: "web", Image: "nginx:latest", Status: "Up 2 hours",
		Ports: "0.0.0.0:8080->80/tcp", RunningFor: "2 hours ago",
		CPUPct: 2.5, MemPct: 12.75, MemUsage: "64MiB / 512MiB",
	}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("Containers() = %#v, want %#v", got, want)
	}
	if calls := runner.calls(); len(calls) != 2 || calls[0] != dockerListCommand || calls[1] != dockerStatsCommand {
		t.Fatalf("commands = %#v", calls)
	}
}

func TestContainersSkipsStatsWhenDockerIsMissing(t *testing.T) {
	t.Parallel()
	// Given a host without docker: the list command answers with the marker.
	runner := &scriptedRunner{replies: map[string]string{dockerListCommand: unsupportedMarker + "\n"}}

	// When the container list is requested.
	_, err := newDiagnosticsCollector(runner).Containers(context.Background(), "web")

	// Then the second round trip is not spent on a host that has no docker.
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Containers() error = %v, want ErrUnsupported", err)
	}
	if calls := runner.calls(); len(calls) != 1 {
		t.Fatalf("commands = %#v, want the stats call skipped", calls)
	}
}

func TestContainersReportsAccessDeniedFromMergedStderr(t *testing.T) {
	t.Parallel()
	// Given a docker socket the current user may not touch: the reason arrives
	// on stderr, which dockerListCommand merges into stdout on purpose.
	denial := "permission denied while trying to connect to the Docker daemon socket"
	runner := &scriptedRunner{replies: map[string]string{
		dockerListCommand:  denial + "\n",
		dockerStatsCommand: "",
	}}

	// When the container list is requested.
	_, err := newDiagnosticsCollector(runner).Containers(context.Background(), "web")

	// Then the missing right is distinguished from a broken host, with the reason kept.
	if !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("Containers() error = %v, want ErrAccessDenied", err)
	}
	if !strings.Contains(err.Error(), denial) {
		t.Fatalf("Containers() error = %q, want the docker reason inside", err)
	}
}

func TestContainersOnHostWithoutContainersReportsNothing(t *testing.T) {
	t.Parallel()
	// Given docker present but no containers at all.
	runner := &scriptedRunner{replies: map[string]string{dockerListCommand: "", dockerStatsCommand: ""}}

	// When the container list is requested.
	got, err := newDiagnosticsCollector(runner).Containers(context.Background(), "web")

	// Then there is neither a list nor an invented failure reason.
	if err != nil || len(got) != 0 {
		t.Fatalf("Containers() = %#v, %v; want empty list without error", got, err)
	}
}

func TestDiagnosticsPropagateTransportFailure(t *testing.T) {
	t.Parallel()
	transport := errors.New("ssh: connection lost")
	tests := []struct {
		name string
		call func(*Collector) error
	}{
		{"processes", func(c *Collector) error { _, err := c.Processes(context.Background(), "web"); return err }},
		{"ports", func(c *Collector) error { _, err := c.Ports(context.Background(), "web"); return err }},
		{"containers", func(c *Collector) error { _, err := c.Containers(context.Background(), "web"); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Given a runner that cannot reach the host.
			runner := &scriptedRunner{err: transport}

			// When a diagnostic is requested.
			err := tt.call(newDiagnosticsCollector(runner))

			// Then the transport failure reaches the caller instead of an empty result.
			if !errors.Is(err, transport) {
				t.Fatalf("error = %v, want %v", err, transport)
			}
		})
	}
}

func TestDiagnosticsStopOnCancelledContext(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		call func(*Collector, context.Context) error
	}{
		{"processes", func(c *Collector, ctx context.Context) error { _, err := c.Processes(ctx, "web"); return err }},
		{"ports", func(c *Collector, ctx context.Context) error { _, err := c.Ports(ctx, "web"); return err }},
		{"containers", func(c *Collector, ctx context.Context) error { _, err := c.Containers(ctx, "web"); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Given a request whose screen was already closed.
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			runner := &scriptedRunner{replies: map[string]string{}}

			// When the diagnostic runs anyway.
			err := tt.call(newDiagnosticsCollector(runner), ctx)

			// Then the cancellation is reported, not parsed as an empty answer.
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context.Canceled", err)
			}
		})
	}
}

func TestDiagnosticsRejectUnknownServer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		call func(*Collector) error
	}{
		{"processes", func(c *Collector) error { _, err := c.Processes(context.Background(), "missing"); return err }},
		{"ports", func(c *Collector) error { _, err := c.Ports(context.Background(), "missing"); return err }},
		{"containers", func(c *Collector) error { _, err := c.Containers(context.Background(), "missing"); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Given a collector that does not know the requested server.
			runner := &scriptedRunner{replies: map[string]string{}}
			collector := newDiagnosticsCollector(runner)

			// When a diagnostic is requested for it.
			err := tt.call(collector)

			// Then the request fails by name without touching a connection.
			if err == nil || !strings.Contains(err.Error(), "missing") {
				t.Fatalf("error = %v, want unknown server name", err)
			}
			if calls := runner.calls(); len(calls) != 0 {
				t.Fatalf("commands = %#v, want none", calls)
			}
		})
	}
}

func TestDiagnosticsCommandsProbeUtilityBeforeUsingIt(t *testing.T) {
	t.Parallel()
	// Given the commands sshmon sends for the three diagnostics.
	tests := []struct {
		name    string
		command string
		want    []string
	}{
		{"processes", processesCommand, []string{"command -v ps", "|| { echo " + unsupportedMarker}},
		{"ports", portsCommand, []string{"command -v ss", "command -v netstat", "else echo " + unsupportedMarker}},
		{"docker list", dockerListCommand, []string{"command -v docker", "|| { echo " + unsupportedMarker, "2>&1"}},
		{"docker stats", dockerStatsCommand, []string{"command -v docker", "|| { echo " + unsupportedMarker}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// When the command text is inspected.
			// Then a host without the utility answers with the marker instead of silence.
			for _, want := range tt.want {
				if !strings.Contains(tt.command, want) {
					t.Fatalf("command %q lost %q", tt.command, want)
				}
			}
		})
	}
}
