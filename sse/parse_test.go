// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package sse_test

import (
	"bufio"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dexpace/go-sdk/sse"
)

func collectEvents(t *testing.T, input string) []sse.Event {
	t.Helper()
	var events []sse.Event
	for ev, err := range sse.Parse(strings.NewReader(input)) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		events = append(events, ev)
	}
	return events
}

func TestParseSingleEvent(t *testing.T) {
	t.Parallel()

	events := collectEvents(t, "data: hello\n\n")
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	got := events[0]
	if got.Type != "message" || got.Data != "hello" {
		t.Fatalf("event = %+v, want Type=message Data=hello", got)
	}
}

func TestParseMultiLineData(t *testing.T) {
	t.Parallel()

	events := collectEvents(t, "data: a\ndata: b\n\n")
	if len(events) != 1 || events[0].Data != "a\nb" {
		t.Fatalf("events = %+v, want one event with Data=\"a\\nb\"", events)
	}
}

func TestParseEventTypeAndStickyID(t *testing.T) {
	t.Parallel()

	input := "event: greeting\nid: 1\ndata: hi\n\ndata: again\n\n"
	events := collectEvents(t, input)
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].Type != "greeting" || events[0].ID != "1" || events[0].Data != "hi" {
		t.Fatalf("event[0] = %+v", events[0])
	}
	if events[1].Type != "message" || events[1].ID != "1" || events[1].Data != "again" {
		t.Fatalf("event[1] = %+v, want Type=message ID=1 Data=again", events[1])
	}
}

func TestParseRetry(t *testing.T) {
	t.Parallel()

	events := collectEvents(t, "retry: 2500\ndata: x\n\n")
	if len(events) != 1 || events[0].Retry != 2500*time.Millisecond {
		t.Fatalf("events = %+v, want Retry=2.5s", events)
	}

	events = collectEvents(t, "retry: soon\ndata: x\n\n")
	if len(events) != 1 || events[0].Retry != 0 {
		t.Fatalf("events = %+v, want Retry=0 for non-numeric", events)
	}
}

func TestParseCommentsAndBlankProduceNoEvents(t *testing.T) {
	t.Parallel()

	if events := collectEvents(t, ": keep-alive\n\n"); len(events) != 0 {
		t.Fatalf("comment+blank produced %d events, want 0", len(events))
	}
	if events := collectEvents(t, "\n\n\n"); len(events) != 0 {
		t.Fatalf("blank lines produced %d events, want 0", len(events))
	}
}

func TestParseCRLF(t *testing.T) {
	t.Parallel()

	events := collectEvents(t, "data: hello\r\n\r\n")
	if len(events) != 1 || events[0].Data != "hello" {
		t.Fatalf("CRLF events = %+v, want Data=hello", events)
	}
}

func TestParseLeadingSpaceStripping(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"data: x\n\n":  "x",
		"data:x\n\n":   "x",
		"data:  x\n\n": " x",
	}
	for input, want := range tests {
		events := collectEvents(t, input)
		if len(events) != 1 || events[0].Data != want {
			t.Fatalf("input %q -> %+v, want Data=%q", input, events, want)
		}
	}
}

func TestParseDiscardsPartialEventAtEOF(t *testing.T) {
	t.Parallel()

	if events := collectEvents(t, "data: incomplete\n"); len(events) != 0 {
		t.Fatalf("got %d events, want 0 (partial discarded at EOF)", len(events))
	}
}

type errReader struct {
	data []byte
	err  error
	done bool
}

func (r *errReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, r.err
	}
	r.done = true
	return copy(p, r.data), nil
}

func TestParseSurfacesReadError(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	r := &errReader{data: []byte("data: x\n\ndata: y"), err: boom}

	var events []sse.Event
	var gotErr error
	for ev, err := range sse.Parse(r) {
		if err != nil {
			gotErr = err
			break
		}
		events = append(events, ev)
	}
	if len(events) != 1 || events[0].Data != "x" {
		t.Fatalf("events = %+v, want one event with Data=x before the error", events)
	}
	if !errors.Is(gotErr, boom) {
		t.Fatalf("err = %v, want boom", gotErr)
	}
}

func TestParseOverLongLine(t *testing.T) {
	t.Parallel()

	long := "data: " + strings.Repeat("a", 2<<20)
	var gotErr error
	for _, err := range sse.Parse(strings.NewReader(long)) {
		if err != nil {
			gotErr = err
			break
		}
	}
	if !errors.Is(gotErr, bufio.ErrTooLong) {
		t.Fatalf("err = %v, want bufio.ErrTooLong", gotErr)
	}
}

func TestParseEarlyBreak(t *testing.T) {
	t.Parallel()

	count := 0
	for range sse.Parse(strings.NewReader("data: a\n\ndata: b\n\n")) {
		count++
		break
	}
	if count != 1 {
		t.Fatalf("consumed %d events, want 1 after break", count)
	}
}
