package collect

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestLogBufferEvictsOldestLinesAtCapacity(t *testing.T) {
	t.Parallel()
	// Given a log buffer with the production line limit.
	buffer := NewLogBuffer(10_000)
	// When more lines arrive than it can retain.
	for i := range 10_005 {
		buffer.Append(fmt.Sprintf("line-%05d", i))
	}
	// Then only the newest ten thousand lines remain.
	got := buffer.Visible()
	if len(got) != 10_000 || got[0] != "line-00005" || got[len(got)-1] != "line-10004" {
		t.Fatalf("unexpected retained range: len=%d first=%q last=%q", len(got), got[0], got[len(got)-1])
	}
}

func TestLogBufferPauseRetainsInputWithoutAdvancingView(t *testing.T) {
	t.Parallel()
	// Given a buffer paused after two visible lines.
	buffer := NewLogBuffer(10)
	buffer.Append("one")
	buffer.Append("two")
	buffer.SetPaused(true)
	// When more lines arrive while paused.
	buffer.Append("three")
	buffer.Append("four")
	// Then the visible window stays fixed, and resuming reveals retained input.
	if got := buffer.Visible(); !reflect.DeepEqual(got, []string{"one", "two"}) {
		t.Fatalf("paused view = %#v", got)
	}
	buffer.SetPaused(false)
	if got := buffer.Visible(); !reflect.DeepEqual(got, []string{"one", "two", "three", "four"}) {
		t.Fatalf("resumed view = %#v", got)
	}
}

func TestLogBufferFilterIsCaseInsensitiveAndReversible(t *testing.T) {
	t.Parallel()
	// Given mixed-case log lines.
	buffer := NewLogBuffer(10)
	buffer.Append("INFO ready")
	buffer.Append("Error database")
	buffer.Append("ERROR network")
	// When a lowercase filter is applied and then cleared.
	buffer.SetFilter("error")
	// Then matching is case-insensitive and clearing restores every line.
	if got := buffer.Visible(); !reflect.DeepEqual(got, []string{"Error database", "ERROR network"}) {
		t.Fatalf("filtered view = %#v", got)
	}
	buffer.SetFilter("")
	if got := buffer.Visible(); len(got) != 3 {
		t.Fatalf("cleared filter retained %d lines", len(got))
	}
}

func TestLogRequestIDsAreDistinct(t *testing.T) {
	t.Parallel()
	// Given two independent live-log requests.
	first := nextLogRequestID()
	second := nextLogRequestID()
	// When their identities are compared.
	// Then stale results can be distinguished by a monotonically unique ID.
	if first == 0 || second == 0 || first == second {
		t.Fatalf("request IDs are not distinct: %d, %d", first, second)
	}
}

func TestLogCommandRejectsUnsafeJournalUnit(t *testing.T) {
	t.Parallel()
	// Given a journal source containing shell metacharacters.
	request := NewLogRequest("web", LogSource{Kind: LogJournal, Name: "ssh.service;rm"})
	// When its read-only command is constructed.
	_, err := (&Collector{}).logCommand(context.Background(), request)
	// Then the untrusted name is rejected before SSH execution.
	if err == nil || !strings.Contains(err.Error(), "недопустимое имя") {
		t.Fatalf("unsafe journal source error = %v", err)
	}
}

func TestLogCommandUsesOnlyKnownKinds(t *testing.T) {
	t.Parallel()
	// Given a log source kind outside the supported closed set.
	request := NewLogRequest("web", LogSource{Kind: LogSourceKind("custom")})
	// When its command is requested.
	_, err := (&Collector{}).logCommand(context.Background(), request)
	// Then callers can branch on the typed unsupported error.
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unsupported source error = %v", err)
	}
}
