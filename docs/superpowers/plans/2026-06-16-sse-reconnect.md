# SSE Reconnecting Stream Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `sse.Stream` — a reconnecting SSE consumer built on `Parse` — with `Last-Event-ID` replay, server-`retry` backoff, and a `ConnectFunc` seam.

**Architecture:** `Stream` loops: call the caller's `ConnectFunc(ctx, lastEventID)`, parse events until the stream ends (EOF/read error), wait the reconnect delay (honoring `ctx`), then reconnect with the latest event id. A connect error is terminal (yielded). An unexported `wait` seam keeps the retry-backoff wiring deterministically testable.

**Tech Stack:** Go 1.26+, standard library only (`context`, `io`, `iter`, `time`). Zero third-party dependencies.

**Conventions every task must follow:**
- MIT license header on every `.go` file before the `package` clause.
- Import groups: stdlib only here.
- Tests: external tests are `package sse_test` (`t.Parallel()`); the internal retry test is `package sse`.
- Tools: Go 1.26.3; `gofumpt`/`golangci-lint` NOT installed — use `gofmt`, `go vet`, `go test -race`.
- Run commands from the repo root `/Users/omar/dexpace/go-sdk`.

---

## File Structure

| Path | Responsibility |
|---|---|
| `sse/stream.go` (new) | `ConnectFunc`, `StreamOption`, `WithReconnectDelay`, `Stream`, unexported `withWait`/`realWait` |
| `sse/stream_test.go` (new, `sse_test`) | reconnect, Last-Event-ID, cancel, connect-error, consumer-break |
| `sse/stream_internal_test.go` (new, `sse`) | retry-overrides-delay via injected wait |
| `sse/doc.go` (modify) | note the reconnecting layer now exists |
| `doc.go`, `README.md` (modify) | update the sse description |

---

## Task 1: `Stream` and reconnection

**Files:**
- Create: `sse/stream.go`, `sse/stream_test.go`, `sse/stream_internal_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// sse/stream_test.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package sse_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/dexpace/go-sdk/sse"
)

func nopBody(s string) io.ReadCloser { return io.NopCloser(strings.NewReader(s)) }

func TestStreamReconnectsAndFlattens(t *testing.T) {
	t.Parallel()

	calls := 0
	connect := func(_ context.Context, _ string) (io.ReadCloser, error) {
		calls++
		switch calls {
		case 1:
			return nopBody("data: a\n\ndata: b\n\n"), nil
		case 2:
			return nopBody("data: c\n\n"), nil
		default:
			return nil, errors.New("stop")
		}
	}

	var data []string
	var gotErr error
	for ev, err := range sse.Stream(context.Background(), connect, sse.WithReconnectDelay(0)) {
		if err != nil {
			gotErr = err
			break
		}
		data = append(data, ev.Data)
	}

	if strings.Join(data, ",") != "a,b,c" {
		t.Fatalf("data = %v, want [a b c]", data)
	}
	if gotErr == nil || gotErr.Error() != "stop" {
		t.Fatalf("err = %v, want stop", gotErr)
	}
}

func TestStreamReplaysLastEventID(t *testing.T) {
	t.Parallel()

	var seenIDs []string
	calls := 0
	connect := func(_ context.Context, lastID string) (io.ReadCloser, error) {
		seenIDs = append(seenIDs, lastID)
		calls++
		if calls == 1 {
			return nopBody("id: 42\ndata: a\n\n"), nil
		}
		return nil, errors.New("stop")
	}

	for _, err := range sse.Stream(context.Background(), connect, sse.WithReconnectDelay(0)) {
		if err != nil {
			break
		}
	}

	if len(seenIDs) != 2 || seenIDs[0] != "" || seenIDs[1] != "42" {
		t.Fatalf("connect lastIDs = %v, want [\"\" \"42\"]", seenIDs)
	}
}

func TestStreamCancellationStops(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	connect := func(_ context.Context, _ string) (io.ReadCloser, error) {
		calls++
		if calls == 1 {
			return nopBody("data: a\n\ndata: b\n\n"), nil
		}
		return nil, errors.New("should not reconnect after cancel")
	}

	var data []string
	for ev, err := range sse.Stream(ctx, connect, sse.WithReconnectDelay(0)) {
		if err != nil {
			t.Fatalf("unexpected error (reconnected after cancel?): %v", err)
		}
		data = append(data, ev.Data)
		if ev.Data == "b" {
			cancel()
		}
	}

	if calls != 1 {
		t.Fatalf("connect called %d times, want 1 (no reconnect after cancel)", calls)
	}
	if strings.Join(data, ",") != "a,b" {
		t.Fatalf("data = %v, want [a b]", data)
	}
}

func TestStreamConnectErrorIsTerminal(t *testing.T) {
	t.Parallel()

	boom := errors.New("dial failed")
	connect := func(_ context.Context, _ string) (io.ReadCloser, error) {
		return nil, boom
	}

	var events, errs int
	for _, err := range sse.Stream(context.Background(), connect, sse.WithReconnectDelay(0)) {
		if err != nil {
			if !errors.Is(err, boom) {
				t.Fatalf("err = %v, want boom", err)
			}
			errs++
			break
		}
		events++
	}
	if events != 0 || errs != 1 {
		t.Fatalf("events=%d errs=%d, want 0 and 1", events, errs)
	}
}

type closeRecorder struct {
	io.Reader
	closed bool
}

func (c *closeRecorder) Close() error { c.closed = true; return nil }

func TestStreamConsumerBreakClosesReader(t *testing.T) {
	t.Parallel()

	rec := &closeRecorder{Reader: strings.NewReader("data: a\n\ndata: b\n\n")}
	calls := 0
	connect := func(_ context.Context, _ string) (io.ReadCloser, error) {
		calls++
		return rec, nil
	}

	for ev := range func(yield func(sse.Event) bool) {
		for e, err := range sse.Stream(context.Background(), connect, sse.WithReconnectDelay(0)) {
			if err != nil {
				return
			}
			if !yield(e) {
				return
			}
		}
	} {
		_ = ev
		break // stop after the first event
	}

	if !rec.closed {
		t.Fatal("reader was not closed on consumer break")
	}
	if calls != 1 {
		t.Fatalf("connect called %d times, want 1", calls)
	}
}
```

```go
// sse/stream_internal_test.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package sse

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestStreamRetryOverridesDelay(t *testing.T) {
	var recorded []time.Duration
	wait := func(ctx context.Context, d time.Duration) bool {
		recorded = append(recorded, d)
		return ctx.Err() == nil
	}

	calls := 0
	connect := func(_ context.Context, _ string) (io.ReadCloser, error) {
		calls++
		if calls == 1 {
			return io.NopCloser(strings.NewReader("retry: 2000\ndata: a\n\n")), nil
		}
		return nil, errors.New("stop")
	}

	var gotErr error
	for _, err := range Stream(context.Background(), connect,
		WithReconnectDelay(time.Hour), withWait(wait)) {
		if err != nil {
			gotErr = err
			break
		}
	}

	if gotErr == nil {
		t.Fatal("expected the stop error")
	}
	if len(recorded) != 1 || recorded[0] != 2000*time.Millisecond {
		t.Fatalf("recorded wait delays = %v, want [2s] (retry overrode the hour default)", recorded)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./sse/ -run Stream -v`
Expected: FAIL — `sse.Stream`/`ConnectFunc`/`WithReconnectDelay`/`withWait` undefined.

- [ ] **Step 3: Create `sse/stream.go`**

```go
// sse/stream.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package sse

import (
	"context"
	"io"
	"iter"
	"time"
)

// defaultReconnectDelay is the wait between reconnects unless overridden by an
// option or a server retry value.
const defaultReconnectDelay = 3 * time.Second

// ConnectFunc opens a fresh event-stream connection. It receives the most recent
// event id (empty on the first connect) so the server can resume via the
// Last-Event-ID request header. Stream closes the returned reader when the
// connection ends.
type ConnectFunc func(ctx context.Context, lastEventID string) (io.ReadCloser, error)

// StreamOption configures [Stream].
type StreamOption func(*streamConfig)

type streamConfig struct {
	delay time.Duration
	wait  func(ctx context.Context, d time.Duration) bool
}

// WithReconnectDelay sets the wait between reconnects. It defaults to three
// seconds and is overridden by any server-sent retry value. A delay <= 0
// reconnects immediately.
func WithReconnectDelay(d time.Duration) StreamOption {
	return func(c *streamConfig) { c.delay = d }
}

// withWait injects the reconnect-wait function. Test seam.
func withWait(fn func(ctx context.Context, d time.Duration) bool) StreamOption {
	return func(c *streamConfig) { c.wait = fn }
}

// Stream yields events from a reconnecting SSE source. It calls connect, parses
// events until the stream ends (EOF or a read error), waits the reconnection
// delay, then reconnects with the most recent event id. A connect error is
// delivered as the iterator error and ends the stream; cancel ctx to stop. The
// iterator is single-pass.
func Stream(ctx context.Context, connect ConnectFunc, opts ...StreamOption) iter.Seq2[Event, error] {
	cfg := streamConfig{delay: defaultReconnectDelay, wait: realWait}
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(yield func(Event, error) bool) {
		lastID := ""
		delay := cfg.delay
		for {
			if ctx.Err() != nil {
				return
			}
			rc, err := connect(ctx, lastID)
			if err != nil {
				yield(Event{}, err)
				return
			}

			stopped := false
			for ev, perr := range Parse(rc) {
				if perr != nil {
					break // mid-stream read error: reconnect transparently
				}
				if ev.ID != "" {
					lastID = ev.ID
				}
				if ev.Retry > 0 {
					delay = ev.Retry
				}
				if !yield(ev, nil) {
					stopped = true
					break
				}
			}
			_ = rc.Close()

			if stopped {
				return
			}
			if !cfg.wait(ctx, delay) {
				return
			}
		}
	}
}

// realWait waits for d, returning false if ctx is (or becomes) done. A
// non-positive d returns true immediately, after the context check.
func realWait(ctx context.Context, d time.Duration) bool {
	if ctx.Err() != nil {
		return false
	}
	if d <= 0 {
		return true
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race ./sse/ -v`
Expected: PASS — all Stream tests (external + internal) plus the existing Parse tests.

- [ ] **Step 5: Update `sse/doc.go`**

Read `sse/doc.go`. Update the package comment to reflect that the reconnecting
layer now exists — replace the "intentionally left to the caller and a future
addition" clause with a sentence pointing at `Stream`:

```go
// Package sse parses a text/event-stream (Server-Sent Events) per the WHATWG
// algorithm, yielding each dispatched [Event] through [Parse] as a range-over-func
// iterator. [Stream] adds a reconnecting consumer over a caller-supplied
// connection, replaying the Last-Event-ID and honoring the server's retry backoff.
package sse
```

- [ ] **Step 6: Commit**

```bash
git add sse/stream.go sse/stream_test.go sse/stream_internal_test.go sse/doc.go
git commit -m "feat(sse): add reconnecting Stream with Last-Event-ID replay"
```

---

## Task 2: docs and full gate

**Files:**
- Modify: `doc.go`, `README.md`

- [ ] **Step 1: Update `doc.go`**

Read `doc.go`. The `package dexpace` comment mentions the sse parser; extend that
sentence (within the single contiguous `//` block) to:

```go
// The sse package parses Server-Sent Events (text/event-stream) into a
// range-over-func iterator of events, with a reconnecting Stream that replays the
// Last-Event-ID.
```
(Replace the existing sse sentence; do not add a second package clause.)

- [ ] **Step 2: Update `README.md`**

Read `README.md`. Update the `sse` row's description to mention the reconnecting
stream: "Server-Sent Events (text/event-stream) WHATWG parser + reconnecting
Stream (Last-Event-ID replay)."

- [ ] **Step 3: Run the full gate**

Run:
```bash
gofmt -l .
go vet ./...
go test -race ./...
```
Expected: `gofmt -l .` prints nothing; `go vet` clean; every package passes under
the race detector.

- [ ] **Step 4: Commit**

```bash
git add doc.go README.md
git commit -m "docs: document the sse reconnecting Stream"
```

---

## Self-Review notes (for the implementer)

- **Spec coverage:** `Stream` + `ConnectFunc` + `WithReconnectDelay` + reconnect
  loop with Last-Event-ID replay and retry backoff (Task 1); docs (Task 2).
- **Type consistency:** `sse.ConnectFunc`, `sse.StreamOption`,
  `sse.WithReconnectDelay`, `sse.Stream`, and the unexported `withWait`/`realWait`
  used identically across tasks/tests.
- **Deterministic tests:** the internal `withWait` recorder asserts the retry
  override without real sleeps; external tests use `WithReconnectDelay(0)` and a
  call-counting `connect`.
- **Resource safety:** the connection reader is closed on every path (consumer
  stop, reconnect); a terminal connect error has no reader to close.
- **Cancellation:** `realWait` checks `ctx` before the zero-delay shortcut, and the
  loop top checks `ctx.Err()`, so a canceled context never triggers a spurious
  reconnect.
- **`make check`** green before opening the PR.
