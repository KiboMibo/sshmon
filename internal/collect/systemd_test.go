package collect

import (
	"context"
	"strings"
	"testing"
)

func TestParseSystemdUnitsParsesPlainListOutput(t *testing.T) {
	t.Parallel()
	// Given plain no-legend systemctl list-units output for two running services.
	raw := "nginx.service loaded active running A high performance web server\n" +
		"docker.service loaded active running Docker Application Container Engine\n"
	// When the output is parsed.
	units := ParseSystemdUnits(raw)
	// Then each line becomes a typed unit with name, states, and description.
	if len(units) != 2 {
		t.Fatalf("units = %#v", units)
	}
	first := units[0]
	if first.Name != "nginx.service" || first.Active != "active" || first.Sub != "running" ||
		first.Description != "A high performance web server" {
		t.Fatalf("first unit = %#v", first)
	}
}

func TestParseSystemdUnitsSkipsMalformedLines(t *testing.T) {
	t.Parallel()
	// Given output polluted by blank and truncated lines.
	raw := "\nbroken line\nnginx.service loaded active running Web server\n"
	// When the output is parsed.
	units := ParseSystemdUnits(raw)
	// Then only the complete unit line survives.
	if len(units) != 1 || units[0].Name != "nginx.service" {
		t.Fatalf("units = %#v", units)
	}
}

func TestSystemdUnitsCommandDiscoversBoundedRunningServicesWhenUnconfigured(t *testing.T) {
	t.Parallel()
	// Given no explicitly configured dashboard units.
	command, err := (&Collector{}).systemdUnitsCommand(nil)
	// When the discovery command is constructed.
	if err != nil {
		t.Fatalf("command error = %v", err)
	}
	// Then it lists only running services, without following, with a hard bound.
	for _, want := range []string{"--type=service", "--state=running", "--no-legend", "head"} {
		if !strings.Contains(command, want) {
			t.Fatalf("command %q misses %q", command, want)
		}
	}
}

func TestSystemdUnitsCommandListsExactConfiguredUnits(t *testing.T) {
	t.Parallel()
	// Given two explicitly configured unit names.
	command, err := (&Collector{}).systemdUnitsCommand([]string{"nginx.service", "postgresql@14.service"})
	// When the listing command is constructed.
	if err != nil {
		t.Fatalf("command error = %v", err)
	}
	// Then both units are requested even when inactive.
	for _, want := range []string{"nginx.service", "postgresql@14.service", "--all"} {
		if !strings.Contains(command, want) {
			t.Fatalf("command %q misses %q", command, want)
		}
	}
}

func TestSystemdUnitsCommandRejectsUnsafeUnitName(t *testing.T) {
	t.Parallel()
	// Given a configured unit name containing shell metacharacters.
	_, err := (&Collector{}).systemdUnitsCommand([]string{"nginx.service;rm -rf /"})
	// When the listing command is constructed.
	// Then the untrusted name is rejected before SSH execution.
	if err == nil || !strings.Contains(err.Error(), "недопустимое имя") {
		t.Fatalf("unsafe unit error = %v", err)
	}
}

func TestLogSnapshotCommandOmitsFollowAndDefaultsToFiftyLines(t *testing.T) {
	t.Parallel()
	// Given a journal snapshot request without an explicit line count.
	request := NewLogRequest("web", LogSource{Kind: LogJournal, Name: "nginx.service"})
	// When the static snapshot command is constructed.
	command, err := (&Collector{}).logSnapshotCommand(context.Background(), request, 0)
	if err != nil {
		t.Fatalf("command error = %v", err)
	}
	// Then it reads a fixed 50-line tail and never follows.
	if !strings.Contains(command, "-n 50") {
		t.Fatalf("command %q misses default line bound", command)
	}
	if strings.Contains(command, "-f") || strings.Contains(command, "-F") {
		t.Fatalf("command %q must not follow", command)
	}
}

func TestLogSnapshotCommandCapsRequestedLines(t *testing.T) {
	t.Parallel()
	// Given a snapshot request demanding far more history than allowed.
	request := NewLogRequest("web", LogSource{Kind: LogSystem})
	// When the static snapshot command is constructed.
	command, err := (&Collector{}).logSnapshotCommand(context.Background(), request, 500)
	if err != nil {
		t.Fatalf("command error = %v", err)
	}
	// Then the tail is capped at 50 lines.
	if !strings.Contains(command, "-n 50") {
		t.Fatalf("command %q misses capped line bound", command)
	}
}

func TestLogSnapshotCommandRejectsUnsafeJournalUnit(t *testing.T) {
	t.Parallel()
	// Given a journal snapshot source containing shell metacharacters.
	request := NewLogRequest("web", LogSource{Kind: LogJournal, Name: "ssh.service;rm"})
	// When the static snapshot command is constructed.
	_, err := (&Collector{}).logSnapshotCommand(context.Background(), request, 50)
	// Then the untrusted name is rejected before SSH execution.
	if err == nil || !strings.Contains(err.Error(), "недопустимое имя") {
		t.Fatalf("unsafe journal source error = %v", err)
	}
}
