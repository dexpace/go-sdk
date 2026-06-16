# Observability — design

**Date:** 2026-06-16
**Status:** Approved (design); ready for implementation planning
**Subsystem:** #3 of the Go SDK platform-parity roadmap

## Context

The SDK has a `logging` policy (structured `log/slog` request/response records) but
no tracing or metrics, and the `instrumentation` package is a placeholder. URL
redaction is duplicated: `logging` has a private `redact()` and `httperr` calls
`url.Redacted()` in two places — and `url.Redacted()` only strips userinfo, so
query-string secrets (`?api_key=…`) still reach logs, traces, and errors.

Java and Python define vendor-neutral observability SPIs (Tracer/Span,
Counter/Histogram) with no-op defaults and ship adapters separately. Go is forced
into the same shape: the de-facto standard (OpenTelemetry) is a third-party
dependency, which core forbids. So we define minimal interfaces, ship no-op
defaults, and let users adapt OTel/Datadog/etc. behind them in their own code.

## Decisions

Settled during design:

1. **Scope:** one spec covering tracing SPI + policy, metrics SPI + policy, and a
   shared URL redactor.
2. **Span model:** a single tracing policy whose granularity is
   placement-determined (like `logging`); default placement is per-attempt
   (inside retry).
3. **Propagation:** the tracing policy injects a W3C `traceparent` header when the
   span context is non-zero and the header is absent.
4. **Metrics:** SPI defines `Histogram` and `UpDownCounter`; the built-in policy
   records a request-duration histogram and an active-requests gauge.
5. **Redaction:** default-deny allowlist — redact all query-param values except an
   explicit allowlist; keys stay visible; userinfo always stripped.
6. **Activation:** `WithTracing(tracer)` / `WithMetrics(meter)` install the
   policies; no-op by default (no policy installed when not configured).

## Architecture

### SPIs and no-op defaults (`instrumentation` package)

```go
// Attr is a key/value attribute attached to spans and metrics.
type Attr struct {
	Key   string
	Value any
}

// SpanContext identifies a span for propagation. The zero value means "no span".
type SpanContext struct {
	TraceID [16]byte
	SpanID  [8]byte
	Sampled bool
}
func (sc SpanContext) IsZero() bool

// Span is a unit of work in a trace.
type Span interface {
	SetAttributes(attrs ...Attr)
	RecordError(err error)
	End()
	Context() SpanContext
}

// Tracer starts spans. StartSpan returns a context carrying the new span so
// nested spans can find their parent.
type Tracer interface {
	StartSpan(ctx context.Context, name string) (context.Context, Span)
}

// Histogram records a distribution (e.g. request duration in seconds).
type Histogram interface {
	Record(ctx context.Context, value float64, attrs ...Attr)
}

// UpDownCounter records an additive value that can rise and fall (e.g. in-flight
// requests).
type UpDownCounter interface {
	Add(ctx context.Context, delta int64, attrs ...Attr)
}

// Meter creates instruments.
type Meter interface {
	Histogram(name string) Histogram
	UpDownCounter(name string) UpDownCounter
}
```

Exported no-ops are the defaults: `NoopTracer` (returns the same context and a
span whose `Context()` is the zero `SpanContext`), `NoopMeter` (instruments whose
methods do nothing). Users write thin adapters mapping these to their backend in
their own code — never in core.

### Shared redactor (`redact` package, new, stdlib-only)

A standalone package so `httperr`, `logging`, and the instrumentation policies can
share it without importing the heavier `instrumentation` package.

```go
type Redactor struct { /* allowed query-param name set */ }

// New builds a redactor whose URL method preserves the listed query-param values
// and redacts all others.
func New(allowedQueryParams ...string) *Redactor

// URL strips userinfo and replaces every non-allowlisted query-param value with
// "REDACTED", preserving keys, path, and fragment. A nil URL yields "".
func (r *Redactor) URL(u *url.URL) string

// Default is the default-deny redactor (empty allowlist): every query value is
// redacted. Used by httperr.
var Default = New()
```

`httperr` switches its two `url.Redacted()` calls to `redact.Default.URL(...)`;
`logging` replaces its private `redact()`; the tracing/metrics policies redact the
URLs they record.

### Tracing policy (`instrumentation.NewTracingPolicy(tracer Tracer, redactor *redact.Redactor) pipeline.Policy`)

Per call:
1. `ctx, span := tracer.StartSpan(req.Context(), req.Method)`; `req.SetContext(ctx)`
   so a second placement nests.
2. `span.SetAttributes` with `http.request.method`, `url.full` (redacted),
   `server.address` (host).
3. **traceparent injection:** if `span.Context()` is non-zero and the request has
   no `traceparent` header, set it to `00-<32-hex traceid>-<16-hex spanid>-<01|00>`.
4. `resp, err := req.Next()`. On error, `span.RecordError(err)`; otherwise
   `span.SetAttributes(http.response.status_code)`. Always `span.End()` (deferred).

Granularity is placement-determined: default install is inside retry (per attempt);
placing it outside retry yields one operation span.

### Metrics policy (`instrumentation.NewMetricsPolicy(meter Meter) pipeline.Policy`)

At construction it builds two instruments from the meter:
`http.client.request.duration` (histogram, seconds) and
`http.client.active_requests` (up-down counter). Per call:
- `inflight.Add(ctx, +1, method)`; `defer inflight.Add(ctx, -1, method)`.
- time `req.Next()`; record duration with attributes `http.request.method` and
  either `http.response.status_code` or an `error=true` attribute.

### Stages and activation

The pipeline stage enum gains `StageTracing` and `StageMetrics`, positioned just
outside the innermost logging stage so the observability trio sits together inside
retry:

```
StageErrors → StageClientIdentity → StageIdempotency → StageRetry → StageAuth →
StageDate → StageTracing → StageMetrics → StageLogging → transport
```

Tracing wraps metrics wraps logging; all per-attempt by default.

Umbrella options:
- `WithTracing(t instrumentation.Tracer)` — installs the tracing policy at
  `StageTracing` (only when `t != nil`).
- `WithMetrics(m instrumentation.Meter)` — installs the metrics policy at
  `StageMetrics` (only when `m != nil`).
- `WithRedactionAllowlist(params ...string)` — client-wide; builds one
  `*redact.Redactor` shared by the logging, tracing, and metrics policies. Default
  (unset) is default-deny. `WithLogging(logger)` keeps its current signature.

`httperr` always uses `redact.Default` (default-deny); it is not configured per
client.

### `pipeline.Request.SetContext`

A small addition so policies can enrich the request context:

```go
// SetContext replaces the underlying request's context. The tracing policy uses
// it to propagate the active span to downstream policies and nested spans.
func (r *Request) SetContext(ctx context.Context)
```

Implemented as `r.req = r.req.WithContext(ctx)`; a nil ctx is ignored.

## Edge cases

- Tracing/metrics policies are installed only when a non-nil tracer/meter is
  supplied; otherwise zero overhead (no policy, no allocation).
- `traceparent` is set only when `SpanContext` is non-zero **and** the header is
  absent — never overriding a caller- or runtime-supplied value, and never set by
  the no-op tracer (its span context is zero).
- The in-flight gauge decrements via `defer`, so it balances on error or panic.
- `Request.SetContext` ignores a nil context (`http.Request.WithContext` panics on
  nil).
- The redactor handles a nil URL (`""`), preserves path and fragment, and matches
  query-param names exactly (case-sensitive, per RFC 3986).
- `span.End()` is deferred so it runs on every path.

## Package layout

| Path | Change |
|---|---|
| `redact/{doc.go,redact.go}` (+ test) | new package |
| `instrumentation/doc.go` | replace placeholder comment |
| `instrumentation/tracer.go` (+ test) | `Tracer`, `Span`, `SpanContext`, `Attr`, no-ops |
| `instrumentation/meter.go` (+ test) | `Meter`, `Histogram`, `UpDownCounter`, no-ops |
| `instrumentation/tracing_policy.go` (+ test) | `NewTracingPolicy` |
| `instrumentation/metrics_policy.go` (+ test) | `NewMetricsPolicy` |
| `pipeline/policy.go` (+ test) | `Request.SetContext` |
| `pipeline/stage.go` (+ test) | `StageTracing`, `StageMetrics` |
| `httperr/httperr.go`, `httperr/transport_error.go` | use `redact.Default` |
| `logging/logging.go` (+ test) | use a `*redact.Redactor` |
| `client.go`, `options.go` (+ test) | `WithTracing`, `WithMetrics`, `WithRedactionAllowlist`; wiring |
| `doc.go`, `README.md` | document |

## Testing

- `redact`: userinfo stripped; non-allowlisted query value → `REDACTED`;
  allowlisted value kept; path/fragment preserved; nil URL → `""`.
- No-ops: `NoopTracer.StartSpan` returns the same context and a span reporting
  `Context().IsZero()`; `NoopMeter` instruments don't panic.
- Tracing policy (fake `Tracer`/`Span` recording calls): span started with the
  method name; attributes set; `End` always called; `RecordError` on error;
  `traceparent` set when the span context is non-zero and absent, NOT overridden
  when present, NOT set when zero.
- Metrics policy (fake `Meter` capturing `Record`/`Add`): duration recorded with
  method/status attributes; in-flight `+1`/`-1` balanced including the error path.
- Umbrella: a fake tracer/meter is invoked for a `Do` when configured; neither
  policy is installed by default; redaction allowlist is plumbed through.
- `pipeline.Request.SetContext`: a downstream policy observes the replaced context.
- Table-driven, parallel (`t.Parallel()`), local fakes, no third-party deps;
  `gofmt`/`go vet`/`go test -race` clean.

## Out of scope (deferred)

- W3C `tracestate` and baggage — only `traceparent`.
- Request/response-size metrics — only duration + in-flight.
- A `ClientLogger` SPI — Go uses `log/slog` directly.
- Concrete OTel/Datadog adapters — live in user code or a future adapter, never
  core.
