# Client-integrated SSE stream — design

**Date:** 2026-06-16
**Status:** Approved (standing delegation); ready for implementation
**Subsystem:** deferred-feature #6 (the `dexpace.Client`-integrated SSE client noted as out-of-scope in the SSE reconnect spec)

## Context

`sse.Stream(ctx, ConnectFunc, …)` reconnects, replays `Last-Event-ID`, and honours
server `retry` backoff, but the caller must wire the HTTP themselves via a
`ConnectFunc`. The common case — "stream this endpoint through my configured
client" — should be one call. This adds `Client.EventStream`, which builds the
`ConnectFunc` from the client pipeline so events flow through auth, logging,
tracing, retry, and the rest exactly like any other request.

## Decisions

1. **A method on `Client`, not a new type.** `EventStream(ctx, req, opts...)
   iter.Seq2[sse.Event, error]` mirrors `Client.Do` (caller supplies a standard
   `*http.Request`) and returns the same iterator shape as `sse.Parse`/`Stream`.
2. **Each (re)connect clones the request** with `req.Clone(ctx)`, sets
   `Accept: text/event-stream` (unless the caller set one), adds the
   `Last-Event-ID` header once an id has been seen, and sends it through `c.Do` —
   so the full policy stack runs per connection (token refresh, per-connect logs).
3. **Connect status check.** A non-2xx response ends the stream with an error
   (`sse.Stream` treats connect errors as terminal); the body is drained and
   closed first. A transport error from `c.Do` is returned as-is.
4. **Replayable bodies.** SSE is normally `GET` (no body). When the request has a
   body, the clone resets it via `req.GetBody` so reconnects re-send it; a
   non-replayable body surfaces an error on the second connect.
5. **`opts ...sse.StreamOption` pass through** to `sse.Stream` (e.g.
   `WithReconnectDelay`).

## Architecture

### `header` addition
`header.LastEventID = "Last-Event-Id"` (canonical form of `Last-Event-ID`).

### `eventstream.go` (package `dexpace`)

```go
// EventStream opens a reconnecting Server-Sent Events stream for req and yields
// decoded events through the client pipeline. Each connection clones req, sets
// Accept: text/event-stream (unless already set) and, after the first event id,
// the Last-Event-ID header, then sends it via Do. A non-2xx response or a
// transport failure on connect ends the stream with that error; a mid-stream
// interruption reconnects transparently with the most recent event id. Cancel the
// request context to stop. The iterator is single-pass.
func (c *Client) EventStream(ctx context.Context, req *http.Request, opts ...sse.StreamOption) iter.Seq2[sse.Event, error]
```

The `ConnectFunc` clones `req` with the connect ctx, resets a replayable body via
`req.GetBody`, sets `Accept` and `Last-Event-ID`, calls `c.Do`, closes the body and
returns an error on a transport failure or non-2xx status, else returns
`resp.Body`.

## Edge cases

- Caller-set `Accept` is preserved (some servers want a custom accept).
- Non-2xx connect → drained, closed, terminal error with the status.
- Transport error → returned; with the default retry policy, transient connect
  failures are already retried inside `c.Do` before `sse.Stream` sees them.
- Body present but non-replayable → error on reconnect (documented; SSE is
  normally bodyless).
- Context cancel → `sse.Stream` stops without a further connect (existing
  behaviour).

## Package layout

| Path | Change |
|---|---|
| `header/header.go` (modify) | add `LastEventID` |
| `eventstream.go` (new, package dexpace) | `Client.EventStream` |
| `eventstream_test.go` (new, `dexpace_test`) | flatten/reconnect, Last-Event-ID, Accept, non-2xx |
| `doc.go`, `README.md` | document |

## Testing

- **Flatten + reconnect**: a stub transporter returns two SSE bodies then a
  terminal error, with `WithReconnectDelay(0)`; assert events `a,b,c` in order
  then the error.
- **Last-Event-ID replay**: events carry ids; assert the header sent on the second
  connect equals the last dispatched id.
- **Accept header**: every connect carries `Accept: text/event-stream`.
- **Non-2xx connect**: a 503 first response yields a terminal error and no events.
- Retry disabled in tests (`WithRetry(retry.Options{MaxRetries: -1})`) for
  deterministic connect counts. Table-driven where natural, parallel; stdlib-only;
  `gofmt`/`go vet`/`go test -race` clean.

## Out of scope (deferred)

- A typed event-decoding helper (callers decode `Event.Data` with `serde`/`jsonl`).
- Automatic `GET`-forcing or body stripping — the caller controls the request.
