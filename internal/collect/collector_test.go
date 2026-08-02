package collect

import (
	"context"
	"strings"
	"testing"
)

func TestPollProbesOSReleaseOnlyOnceWhenFileIsMissing(t *testing.T) {
	// Given a host whose sample never carries an @@OS section: on busybox,
	// OpenWrt and part of the container images /etc/os-release does not exist.
	runner := &fakePollRunner{output: rawFixture}
	collector := newReconnectTestCollector("web", runner)

	// When the host is polled twice.
	for range 2 {
		if err := collector.poll(context.Background(), collector.states[0]); err != nil {
			t.Fatalf("poll() error = %v", err)
		}
	}

	// Then the probe went out with the first sample only.
	if len(runner.commands) != 2 {
		t.Fatalf("commands = %#v", runner.commands)
	}
	if !strings.Contains(runner.commands[0], "@@OS") {
		t.Fatalf("first sample lost the OS probe: %q", runner.commands[0])
	}
	if strings.Contains(runner.commands[1], "@@OS") {
		t.Fatalf("second sample still probes the OS: %q", runner.commands[1])
	}
}
