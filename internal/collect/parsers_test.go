package collect

import (
	"errors"
	"testing"
)

func TestParseProcessesSupportsGNUAndBusyBox(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  string
		want Process
	}{
		{
			name: "gnu ps",
			raw:  "  123 12.5 3.2 /usr/bin/nginx -g daemon off;\nmalformed\n",
			want: Process{PID: 123, Command: "/usr/bin/nginx -g daemon off;", CPUPct: 12.5, MemPct: 3.2},
		},
		{
			name: "busybox ps",
			raw:  "PID USER VSZ STAT COMMAND\n7 root 1234 S /sbin/init\n",
			want: Process{PID: 7, Command: "/sbin/init"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given process output from a supported ps implementation.
			// When it is parsed.
			got, err := ParseProcesses(tt.raw)
			// Then malformed/header lines are skipped and the process fields survive.
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 || got[0] != tt.want {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParseContainersCombinesListAndStats(t *testing.T) {
	t.Parallel()
	// Given read-only docker list (now with ports column) and one-shot stats output with a malformed row.
	list := "abc123\tweb\tnginx:latest\tUp 2 hours\t0.0.0.0:8080->80/tcp\nbad\n"
	stats := "abc123\t2.50%\t12.75%\t64MiB / 512MiB\n"
	// When both outputs are parsed.
	got, err := ParseContainers(list, stats)
	// Then status, resource usage, and exposed ports are combined by container ID.
	if err != nil {
		t.Fatal(err)
	}
	want := Container{ID: "abc123", Name: "web", Image: "nginx:latest", Status: "Up 2 hours", Ports: "0.0.0.0:8080->80/tcp", CPUPct: 2.5, MemPct: 12.75, MemUsage: "64MiB / 512MiB"}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestParsePortsPreservesProcessAndPID(t *testing.T) {
	t.Parallel()
	// Given ss output with TCP/UDP listeners and one malformed row.
	raw := "tcp LISTEN 0 4096 0.0.0.0:22 0.0.0.0:* users:((\"sshd\",pid=123,fd=3))\n" +
		"udp UNCONN 0 0 0.0.0.0:68 0.0.0.0:* users:((\"dhclient\",pid=77,fd=6))\nnope\n"
	// When ports are parsed.
	got, err := ParsePorts(raw)
	// Then protocol, local address, process and PID are retained.
	if err != nil {
		t.Fatal(err)
	}
	want := []Port{{Proto: "tcp", Local: "0.0.0.0:22", Process: "sshd", PID: 123}, {Proto: "udp", Local: "0.0.0.0:68", Process: "dhclient", PID: 77}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestDiagnosticsParsersReturnUnsupportedMarker(t *testing.T) {
	t.Parallel()
	// Given the marker emitted when a remote command is unavailable.
	// When each diagnostics parser receives it.
	_, processErr := ParseProcesses(unsupportedMarker)
	_, containerErr := ParseContainers(unsupportedMarker, "")
	_, portErr := ParsePorts(unsupportedMarker)
	// Then callers can branch on the typed unsupported error.
	for _, err := range []error{processErr, containerErr, portErr} {
		if !errors.Is(err, ErrUnsupported) {
			t.Fatalf("got %v, want ErrUnsupported", err)
		}
	}
}
