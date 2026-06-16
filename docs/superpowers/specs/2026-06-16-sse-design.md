# SSE (Server-Sent Events) — design

**Date:** 2026-06-16
**Status:** Approved (standing delegation); ready for implementation planning
**Subsystem:** #9 of the Go SDK platform-parity roadmap

## Context

The `sse` package is a placeholder. Java/Python ship a WHATWG event-stream parser
and a reconnecting connection (Last-Event-ID replay, server `retry` backoff). This
subsystem delivers the parser core; the reconnecting layer is deferred.

## Decisions

1. **Scope: the WHATWG parser.** `Event` plus `Parse(r io.Reader) iter.Seq2[Event, error]`
   implementing the WHATWG "event stream interpretation" algorithm.
2. **Idiomatic streaming.** Events are yielded through a range-over-func iterator,
   matching `pagination`.
3. **Bounded lines.** The scanner buffer is capped (`maxLineBytes`); an over-long
   line yields an error rather than growing without bound.
4. **Deferred (documented): the reconnecting connection.** Last-Event-ID replay,
   server `retry` honoring, and reconnect/backoff are a resilience layer that
   needs timer-driven control flow; it lands in a follow-up. The parser is fully
   usable on its own (a caller can reconnect by re-invoking `Parse` on a fresh
   stream).

## Architecture

### `Event` and `Parse` (`sse` package, stdlib-only)

```go
// Event is one Server-Sent Event.
type Event struct {
	// Type is the event type ("message" when the stream did not specify one).
	Type string
	// Data is the event payload with its trailing newline removed; multi-line
	// data fields are joined with "\n".
	Data string
	// ID is the last seen event id (sticky across events, per the WHATWG spec).
	ID string
	// Retry is the reconnection-time hint from a retry field on this event, or 0
	// when none was given.
	Retry time.Duration
}

// Parse interprets r as a text/event-stream and yields each dispatched event.
// It follows the WHATWG event-stream algorithm: data/event/id/retry fields are
// accumulated and an event is dispatched on a blank line. Lines may end with LF
// or CRLF. Comment lines (beginning with ":") are ignored. A read error (or an
// over-long line) is delivered as the second iteration value, after which
// iteration stops. The iterator is single-pass.
func Parse(r io.Reader) iter.Seq2[Event, error]
```

### Algorithm (WHATWG event-stream interpretation)

Line scanning uses `bufio.Scanner` (LF and CRLF; bare-CR-only terminators — vanishingly
rare — are not split and are documented as unsupported). Buffers: `data` (a
`strings.Builder`), `eventType` (string), `lastID` (string, sticky).

Per line:
- **empty line** → dispatch: if the `data` buffer is non-empty, yield an `Event`
  with `Type = eventType or "message"`, `Data = data` with one trailing `"\n"`
  removed, `ID = lastID`, `Retry = retry` (the value parsed for this event, else
  0). Reset `data`, `eventType`, and the per-event `retry` (NOT `lastID`). If the
  `data` buffer is empty, reset `eventType`/`retry` and continue without yielding.
- **line starting with `:`** → comment, ignored.
- **field line** → split on the first `:`; the value has at most one leading space
  stripped:
  - `data` → append `value + "\n"` to the `data` buffer.
  - `event` → `eventType = value`.
  - `id` → if the value contains no NUL, `lastID = value`.
  - `retry` → if the value is all ASCII digits, set this event's `retry` to that
    many milliseconds.
  - any other field name → ignored.
- **EOF** → a partially-accumulated (never blank-line-terminated) event is
  discarded, per the spec.

`maxLineBytes` caps the scanner buffer; `bufio.ErrTooLong` is surfaced as the
iteration error.

## Edge cases

- `data:` with no value → appends `"\n"` (an empty data line is still data).
- Multiple `data:` lines → joined with `"\n"`; the final trailing `"\n"` is
  stripped on dispatch (so two `data: a` / `data: b` lines → `"a\nb"`).
- `id` is sticky: an event without an `id` field keeps the previous `lastID`.
- `id` containing a NUL byte is ignored (lastID unchanged), per spec.
- `retry: abc` (non-numeric) is ignored.
- A blank line with no preceding data dispatches nothing.
- A comment-only line (`: keep-alive`) is ignored and does not dispatch.
- Leading-space stripping: `data: x` → `x`; `data:x` → `x`; `data:  x` → `" x"`.
- Read error mid-stream → yielded once, then iteration stops; any partial event is
  discarded.
- Early `break` from the iterator stops scanning (range-over-func semantics).

## Package layout

| Path | Change |
|---|---|
| `sse/doc.go` (modify) | real package comment (note the deferred reconnect layer) |
| `sse/event.go` (new) | the `Event` type |
| `sse/parse.go` (new) | `Parse` + the WHATWG interpreter |
| `sse/parse_test.go` (new) | algorithm tests |
| `doc.go`, `README.md`, `CLAUDE.md` | document; de-placeholder `sse` |

## Testing

- Single event: `data: hello\n\n` → one event, `Type=message`, `Data=hello`.
- Multi-line data joined with `\n`; trailing newline stripped.
- `event:` sets the type; `id:` is sticky across events; `retry:` parsed to a
  duration (and non-numeric ignored).
- Comment lines and blank-line-only input dispatch nothing.
- CRLF line endings parse identically to LF.
- Leading-space stripping rules.
- An over-long line yields an error.
- A mid-stream read error (a fake reader) is surfaced and ends iteration.
- Early `break` stops consuming.
- Table-driven, parallel; stdlib-only; `gofmt`/`go vet`/`go test -race` clean.

## Out of scope (deferred)

- Reconnecting connection: Last-Event-ID replay, server `retry` honoring,
  reconnect with backoff. (Follow-up subsystem.)
- Bare-CR-only line terminators (documented as unsupported; no real server uses
  them).
- An SSE client integrated with `dexpace.Client` (the parser takes any `io.Reader`).
