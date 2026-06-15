# Error model — design

**Date:** 2026-06-15
**Status:** Approved (design); ready for implementation planning
**Subsystem:** #2 of the Go SDK platform-parity roadmap

## Context

The Go SDK currently has a thin error surface:

- `httperr.ResponseError` represents a non-2xx response, but it is **opt-in** —
  `Client.Do` mirrors `http.Client.Do` and returns `(*http.Response, nil)` for a
  404; the caller chooses to call `httperr.FromResponse(resp)`.
- **Transport failures** (the request never produced a response — DNS,
  connection refused, TLS, timeout) surface as **raw `net/http` errors**
  (`*url.Error`). The `retry` policy classifies them inline.
- There is **no** transport-error type and **no** unified activation of the
  typed model.

Java and Python expose a richer taxonomy (transport-failure error, response
error carrying a decoded body, timeout variants, a shared retryability notion).
This subsystem brings the useful parts to Go **idiomatically**, honouring the
SDK's defining constraint — lean on `net/http` — by keeping the typed model
**opt-in** and the defaults pure.

## Decisions

Settled during design; not open for re-litigation in the plan:

1. **Status errors are opt-in.** A single `dexpace.WithErrors()` option installs
   the typed model. With no opt-in, `Client.Do` behaves exactly as today (raw
   errors; `404 → (resp, nil)`).
2. **Thin typed transport wrapper.** `httperr.TransportError` wraps the
   underlying `net/http` cause; `Unwrap()` preserves `errors.Is/As` to
   `*url.Error`, `net.Error`, and context errors.
3. **One opt-in switch, both behaviours.** `WithErrors()` enables BOTH non-2xx →
   `ResponseError` and transport-error wrapping. Both are off by default.
4. **Retryability stays in `retry`.** The error model is types + helpers only.
   Because `TransportError.Unwrap` is lossless, `retry`'s existing `errors.Is/As`
   checks work whether or not `WithErrors()` is enabled; `retry` does not import
   `httperr`.

## Architecture

### Placement: the error mapper is the OUTERMOST policy

The critical constraint: the status-error policy must sit **outside** retry. If
it converted a 503 into a `ResponseError` *inside* retry, retry would see
`err != nil, resp == nil` and lose its status-code logic — it would retry every
error by method instead of by the configured status codes (so a 404 would
wrongly retry).

Therefore the error mapper is the outermost policy. Retry runs on raw
responses/errors during its attempts; only the **final** result is mapped to the
typed model. Transport-error wrapping consequently happens after retries are
exhausted, which is correct.

- New `pipeline.StageErrors` constant — the new outermost stage (lower ordinal
  than `StageClientIdentity`).
- `dexpace.WithErrors()` installs one inline `errorsPolicy` at `StageErrors`.
- The policy calls `Next()` (running retry → … → transport), then maps the final
  result:
  - `err != nil` and not a context error → `httperr.FromError(err, req)` →
    `*TransportError`.
  - `resp != nil && resp.StatusCode >= 400` → `httperr.FromResponse(resp)` →
    `*ResponseError`.
- Context `Canceled`/`DeadlineExceeded` (even wrapped in `*url.Error`) pass
  through unwrapped — they are the caller's deadline, not a transport fault — so
  `errors.Is(err, context.Canceled)` keeps working.

### Types and helpers (`httperr`, kept pure: types + functions, stdlib-only)

```go
// TransportError reports that a request never produced a response (DNS,
// connection, TLS, or network timeout). It wraps the underlying net/http cause.
type TransportError struct {
	Method string // request method
	URL    string // redacted request URL
	Err    error  // the underlying cause
}

func (e *TransportError) Error() string // "transport error: GET https://…: <cause>"
func (e *TransportError) Unwrap() error // returns Err — preserves errors.Is/As
func (e *TransportError) Timeout() bool // true if the cause is a net.Error timeout
```

`ResponseError` is unchanged (`StatusCode`, `Status`, `Method`, `URL`,
`RawResponse`, `Body()`).

Helpers (usable standalone by low-level `pipeline.New` users and called by the
inline `errorsPolicy`):

```go
func FromResponse(resp *http.Response) *ResponseError // exists; non-2xx → error, else nil
func FromError(err error, req *http.Request) error    // new: nil→nil; context error→err unchanged;
                                                       // else → *TransportError with redacted method/URL
```

`Timeout()` uses `errors.As(e.Err, &netErr)` and returns `netErr.Timeout()`;
false for non-net causes.

### Return convention on a status error

The policy returns `(resp, rerr)` — both non-nil. `FromResponse` has already
buffered (≤ 8 KiB) and rewound the body, so the caller may read `resp.Body`,
ignore it safely, or use `rerr.Body()` / `rerr.RawResponse`. This matches the
existing `FromResponse` semantics rather than nulling the response.

The `errorsPolicy` lives inline in the umbrella package (like `datePolicy`),
calling the two helpers, so `httperr` never imports `pipeline`.

## Edge cases

- Context `Canceled`/`DeadlineExceeded`, even wrapped in `*url.Error`, are
  returned unwrapped and never converted to `TransportError`.
- A non-nil response together with a transport error should not occur; if it
  does, the error wins and `resp.Body` is drained/closed to avoid a leak.
- `TransportError.Timeout()` returns false for non-`net.Error` causes.
- An oversized/streaming error body is capped at 8 KiB by the existing
  `FromResponse` drain (documented truncation).
- `WithErrors()` is idempotent and order-independent among options.

## Package layout

| Path | Change |
|---|---|
| `httperr/transport_error.go` (+ test) | new — `TransportError`, `FromError` |
| `pipeline/stage.go` | add `StageErrors` (new outermost); update doc |
| `pipeline/doc.go` | mention `StageErrors` in the stage list |
| `client.go` | inline `errorsPolicy()`; wire `At(StageErrors, …)` when enabled |
| `options.go` | `errors bool` field + `WithErrors()` |
| `client_test.go` | activation + behaviour tests |

## Testing

- Table-driven, parallel (`t.Parallel()`); local `transporterFunc` fakes;
  response bodies closed via `t.Cleanup`.
- `WithErrors()` **off** → behaviour unchanged: `404 → (resp, nil)`; a transport
  failure → raw `*url.Error` (assert `errors.As` to `*url.Error`, and that it is
  NOT a `*httperr.TransportError`).
- `WithErrors()` **on**: a 404 yields `*ResponseError` (via `errors.As`); a
  connection failure yields `*TransportError` whose `Unwrap` reaches the cause
  and whose `Timeout()` is correct; context cancellation passes through
  unwrapped.
- **Retry interaction:** with `WithErrors()` + retry, a 503 is retried N times
  (retry sees raw responses), then the final result is a `*ResponseError`. A
  keyed/idempotent transport failure is retried, then surfaces as a
  `*TransportError`.
- Unit tests for `FromError` (nil, context error, generic error) and
  `TransportError` (`Error`, `Unwrap`, `Timeout`).
- No new third-party dependencies; `gofmt`/`go vet`/`go test -race` clean.

## Out of scope (deferred)

- **Typed/generic decoded error bodies** (Java/Python `HttpResponseError[T]`) —
  needs `serde` (Tier 2). `ResponseError.Body()` stays raw bytes; a `DecodeInto`
  hook lands when serde exists.
- Separate timeout **types** — the `Timeout()` helper covers it.
- `ErrorMap` (status→custom-error registry) — revisit only on real demand.
