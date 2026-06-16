# SSE reconnecting stream — design

**Date:** 2026-06-16
**Status:** Approved (standing delegation); ready for implementation planning
**Subsystem:** deferred-feature #2 (the reconnecting connection from the SSE roadmap item)

## Context

The `sse` package parses an event stream from an `io.Reader`. The deferred
resilience layer — auto-reconnect with `Last-Event-ID` replay and server `retry`
backoff — completes parity with the Java/Python SDKs. This adds it as
`sse.Stream`, built on the existing `Parse`.

## Decisions

1. **Caller supplies the connection.** A `ConnectFunc(ctx, lastEventID)` returns a
   fresh `io.ReadCloser` event stream. This keeps `sse` decoupled from
   `dexpace.Client` (the caller wires whatever HTTP they use) and makes
   `Last-Event-ID` replay explicit.
2. **Transparent reconnect on stream end.** When a connected stream ends (EOF or a
   mid-stream read error), `Stream` waits the reconnection delay and reconnects
   with the most recent event id. A **connect** error is terminal (yielded), so a
   persistent inability to connect does not loop forever.
3. **Server-driven backoff.** The delay starts at a configurable default
   (`WithReconnectDelay`, default 3s) and is updated by any `retry` field the
   stream sends.
4. **Deterministic and cancelable.** The iterator is pull-based; the reconnect
   wait checks `ctx` first (so a canceled context stops without a spurious
   reconnect) and `WithReconnectDelay(0)` makes tests fast.

## Architecture

### `sse.Stream` (added to the `sse` package)

```go
// ConnectFunc opens a fresh event-stream connection. It receives the most recent
// event id (empty on the first connect) so the server can resume via the
// Last-Event-ID request header. The returned reader is closed by Stream when the
// connection ends.
type ConnectFunc func(ctx context.Context, lastEventID string) (io.ReadCloser, error)

// StreamOption configures Stream.
type StreamOption func(*streamConfig)

// WithReconnectDelay sets the wait between reconnects. It defaults to three
// seconds and is overridden by any server-sent retry value. A delay <= 0
// reconnects immediately (useful in tests).
func WithReconnectDelay(d time.Duration) StreamOption

// Stream yields events from a reconnecting SSE source. It calls connect, parses
// events until the stream ends (EOF or a read error), waits the reconnection
// delay, then reconnects with the most recent event id. A connect error is
// delivered as the iterator error and ends the stream; cancel ctx to stop. The
// iterator is single-pass.
func Stream(ctx context.Context, connect ConnectFunc, opts ...StreamOption) iter.Seq2[Event, error]
```

### Behaviour

- Track `lastID` (carried across reconnects, sticky) and `delay`
  (default → updated by `retry`).
- Loop:
  1. If `ctx` is done, stop.
  2. `rc, err := connect(ctx, lastID)`. On error → `yield(Event{}, err)` and stop.
  3. For each `(ev, perr)` from `Parse(rc)`:
     - `perr != nil` (mid-stream read error) → close `rc`, break to reconnect
       (the error is not surfaced; reconnection is the SSE contract).
     - else: if `ev.ID != ""` set `lastID`; if `ev.Retry > 0` set `delay`;
       `yield(ev, nil)`. If the consumer stops (`yield` returns false) → close `rc`
       and stop.
  4. The stream ended (EOF or read error). Close `rc`. Wait `delay`, honoring
     `ctx`; if `ctx` is canceled during the wait, stop. Otherwise reconnect.
- The wait helper returns immediately false if `ctx` is already done (so a
  canceled context never triggers another connect), returns true for a
  non-positive delay, otherwise selects on a timer vs `ctx.Done()`.

## Edge cases

- A connect error on the **first** attempt → yielded immediately, stream ends.
- An empty stream (connect returns a reader that EOFs with no events) → reconnect
  after the delay (a heartbeat-less keep-open).
- The consumer breaking out of the range stops fetching and closes the current
  reader (no further connects).
- `Last-Event-ID` replay: the id of the most recent dispatched event is passed to
  the next `connect`; an event without an id keeps the prior id (sticky, from
  `Parse`).
- A server `retry` value updates the delay for all subsequent reconnects.
- `WithReconnectDelay(0)` reconnects with no wait but still stops on a canceled
  context (the wait helper checks `ctx` before the zero-delay shortcut).
- Each connection's reader is always closed (on consumer stop, on reconnect, on
  terminal connect error there is no reader to close).

## Package layout

| Path | Change |
|---|---|
| `sse/stream.go` (new) | `ConnectFunc`, `StreamOption`, `WithReconnectDelay`, `Stream`, unexported `withWait` + `realWait` |
| `sse/stream_test.go` (new, `sse_test`) | reconnect, Last-Event-ID, cancel, connect-error, consumer-break tests |
| `sse/stream_internal_test.go` (new, `sse`) | retry-overrides-delay via injected `withWait` recorder |
| `sse/doc.go` (modify) | note that `Stream` now provides the reconnecting layer |
| `doc.go`, `README.md` | update the sse description |

## Testing

- Reconnect + flatten: `connect` returns successive in-memory streams (events
  then EOF), with `WithReconnectDelay(0)`; the third connect returns an error to
  stop. Assert all events arrive in order, then the connect error.
- `Last-Event-ID` replay: events carry ids; assert the id passed to the second
  `connect` equals the last dispatched id from the first stream.
- Retry backoff (deterministic, internal test): an unexported `withWait(fn)`
  option injects a recording wait. A stream sends `retry: 2000` then EOF; the
  next connect returns a stop-error. The internal test asserts the recorded wait
  delay equals 2000ms (the server `retry` overrode the default). The wait seam is
  unexported (no public API addition) and used only by `package sse` tests.
- Cancellation: cancel `ctx` after consuming the first stream's events; assert no
  further `connect` call and the iterator ends.
- Connect error first attempt → yielded, no events.
- Consumer break → only the consumed events fetched; the reader is closed
  (a reader that records Close).
- Table-driven where natural, parallel where no shared env; stdlib-only; `gofmt`/
  `go vet`/`go test -race` clean.

## Out of scope (deferred)

- A `dexpace.Client`-integrated SSE client (the `ConnectFunc` seam lets callers
  wire the client themselves).
- Exponential backoff / jitter on repeated connect failures (connect errors are
  terminal here; add a retry-on-connect policy later if needed).
