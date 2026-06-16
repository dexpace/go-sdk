# JSONL / NDJSON stream decoder — design

**Date:** 2026-06-16
**Status:** Approved (standing delegation); ready for implementation planning
**Subsystem:** deferred-feature #3 (JSONL/chunked stream helpers from the HTTP-value-types roadmap item)

## Context

Streaming JSON APIs return newline-delimited JSON (NDJSON / JSON Lines): one JSON
value per chunk. Consuming them by hand (decoder loop, EOF handling) is repetitive.
This package decodes such a stream into typed values via a range-over-func
iterator, matching `pagination` and `sse`.

## Decisions

1. **Generic stream decoder.** `Decode[T](r io.Reader) iter.Seq2[T, error]` yields
   each successive JSON value decoded into a `T`.
2. **Lean on `encoding/json`.** A `json.Decoder` reads successive values from the
   stream, tolerating any whitespace (including newlines) between them — so NDJSON,
   pretty-printed concatenation, and a single value all work. No custom line
   splitting.
3. **Bounded by the decoder.** `json.Decoder` reads incrementally; the package
   adds no unbounded buffering of the whole stream.
4. **Own small package `jsonl`.** Named for what it provides; discoverable at the
   call site (`jsonl.Decode`).

## Architecture

### `jsonl` package (stdlib-only)

```go
// Decode reads a stream of JSON values from r and yields each decoded into a T.
// Values may be separated by any JSON whitespace (newlines for NDJSON / JSON
// Lines, or none). Iteration ends at end of stream; a decode error is delivered
// as the second iteration value, after which iteration stops. The iterator is
// single-pass.
func Decode[T any](r io.Reader) iter.Seq2[T, error]
```

Implementation: a `json.Decoder` over `r`; loop `dec.Decode(&v)`; `io.EOF` ends
iteration cleanly; any other error is yielded once (with the zero `T`) and stops;
each successfully decoded value is yielded; a consumer break stops decoding.

## Edge cases

- An empty stream → no values, no error.
- Trailing whitespace/newline after the last value → clean EOF, no spurious value.
- A malformed value mid-stream → the decode error is yielded once, then iteration
  stops (no attempt to resynchronize).
- A partial value at EOF (truncated stream) → `json.Decoder` returns an
  `io.ErrUnexpectedEOF`-class error, yielded as the iteration error.
- Early `break` from the iterator stops decoding (range-over-func semantics).
- The element type may be any JSON-decodable type (struct, map, slice, scalar).

## Package layout

| Path | Change |
|---|---|
| `jsonl/doc.go` (new) | package comment |
| `jsonl/jsonl.go` (new) | `Decode` |
| `jsonl/jsonl_test.go` (new) | NDJSON, single value, empty, error, break tests |
| `doc.go`, `README.md`, `CLAUDE.md` | document; add the package |

## Testing

- NDJSON: three `{"n":...}` objects on separate lines → three values in order.
- Whitespace tolerance: values separated by extra spaces/newlines decode the same;
  a single value (no trailing newline) decodes.
- Scalars: a stream of bare numbers (`1 2 3`) decodes into `int`s.
- Empty stream → zero values, no error.
- A malformed value mid-stream → the first values decode, then the error is
  yielded and iteration stops.
- Truncated final value → an error is yielded.
- Early `break` stops decoding (a counting reader or asserting only N consumed).
- Table-driven where natural, parallel; stdlib-only; `gofmt`/`go vet`/`go test
  -race` clean.

## Out of scope (deferred)

- Chunked-transfer framing helpers (net/http already de-chunks the body; the
  decoded stream is what `Decode` consumes).
- An `Encode` (NDJSON writer) — callers can `json.Marshal` + write a newline; add
  if a real need appears.
- Resynchronizing after a malformed value (skip-and-continue) — stops on first
  error, matching the other iterators.
