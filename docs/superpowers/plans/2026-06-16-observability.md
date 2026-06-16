# Observability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add vendor-neutral tracing and metrics SPIs (with no-op defaults and pipeline policies) plus a shared default-deny URL redactor, activated opt-in via `WithTracing`/`WithMetrics`.

**Architecture:** A new `redact` package centralizes URL redaction (userinfo + default-deny query values). A new `instrumentation` API defines `Tracer`/`Span`/`Meter` interfaces with no-op defaults and two pipeline policies (tracing, metrics). The tracing policy injects a W3C `traceparent` header and propagates the span via a new `pipeline.Request.SetContext`. Two new stages (`StageTracing`, `StageMetrics`) host the policies inside retry by default; the umbrella installs them only when a tracer/meter is supplied.

**Tech Stack:** Go 1.26+, standard library only (`context`, `net/http`, `net/url`, `sort`, `strings`, `fmt`, `time`, `log/slog`). Zero third-party runtime dependencies.

**Conventions every task must follow:**
- MIT license header on every `.go` file (src and tests), before the `package` clause:
  ```go
  // Copyright (c) 2026 dexpace and Omar Aljarrah.
  // Licensed under the MIT License. See LICENSE in the repository root for details.
  ```
- Import groups: stdlib, blank line, then `github.com/dexpace/go-sdk/...` (alphabetized).
- Tests use `t.Parallel()`; response bodies closed via `t.Cleanup`. Local fakes, no mocking frameworks.
- Tools: Go 1.26.3 installed; `gofumpt`/`goimports`/`golangci-lint` NOT installed locally — use `gofmt`, `go vet`, `go test -race`; CI runs linters.
- Run commands from the repo root `/Users/omar/dexpace/go-sdk`.

---

## File Structure

| Path | Responsibility |
|---|---|
| `redact/{doc.go,redact.go}` (+ test) | new package: `Redactor`, `New`, `URL`, `Default` |
| `pipeline/policy.go` (modify) + test | `Request.SetContext` |
| `pipeline/stage.go` (modify) + test | `StageTracing`, `StageMetrics` |
| `instrumentation/doc.go` (modify) | real package comment |
| `instrumentation/tracer.go` (+ test) | `Tracer`, `Span`, `SpanContext`, `Attr`, no-ops |
| `instrumentation/meter.go` (+ test) | `Meter`, `Histogram`, `UpDownCounter`, no-ops |
| `instrumentation/tracing_policy.go` (+ test) | `NewTracingPolicy` |
| `instrumentation/metrics_policy.go` (+ test) | `NewMetricsPolicy` |
| `httperr/httperr.go`, `httperr/transport_error.go` (+ test) | use `redact.Default` |
| `logging/logging.go` (+ test) | use `*redact.Redactor` |
| `client.go`, `options.go` (+ test) | `WithTracing`, `WithMetrics`, `WithRedactionAllowlist`; wiring |
| `doc.go`, `README.md` | document |

---

## Task 1: `redact` package

**Files:**
- Create: `redact/doc.go`, `redact/redact.go`
- Test: `redact/redact_test.go`

- [ ] **Step 1: Write the failing test**

```go
// redact/redact_test.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package redact_test

import (
	"net/url"
	"testing"

	"github.com/dexpace/go-sdk/redact"
)

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return u
}

func TestDefaultRedactsUserinfoAndAllQueryValues(t *testing.T) {
	t.Parallel()

	u := mustURL(t, "https://user:pass@api.example.test/things?api_key=secret&page=2")
	got := redact.Default.URL(u)

	want := "https://api.example.test/things?api_key=REDACTED&page=REDACTED"
	if got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}
}

func TestAllowlistKeepsListedValues(t *testing.T) {
	t.Parallel()

	r := redact.New("page")
	u := mustURL(t, "https://api.example.test/things?api_key=secret&page=2")
	got := r.URL(u)

	want := "https://api.example.test/things?api_key=REDACTED&page=2"
	if got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}
}

func TestNilURL(t *testing.T) {
	t.Parallel()

	if got := redact.Default.URL(nil); got != "" {
		t.Fatalf("URL(nil) = %q, want empty", got)
	}
}

func TestPreservesPathAndFragmentNoQuery(t *testing.T) {
	t.Parallel()

	u := mustURL(t, "https://api.example.test/a/b#frag")
	if got := redact.Default.URL(u); got != "https://api.example.test/a/b#frag" {
		t.Fatalf("URL = %q, want path and fragment preserved", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./redact/ -v`
Expected: FAIL — package/`redact.New` undefined.

- [ ] **Step 3: Write the implementation**

```go
// redact/doc.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package redact renders log- and trace-safe representations of URLs. It strips
// userinfo and, by default, every query-string value, so secrets carried in a URL
// (API keys, tokens) never reach logs, traces, or error messages. A configurable
// allowlist keeps chosen query-param values visible.
package redact
```

```go
// redact/redact.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package redact

import (
	"net/url"
	"sort"
	"strings"
)

// redactedValue replaces a non-allowlisted query-param value.
const redactedValue = "REDACTED"

// Redactor renders URLs with userinfo stripped and non-allowlisted query-param
// values replaced by "REDACTED". A Redactor is safe for concurrent use.
type Redactor struct {
	allowed map[string]struct{}
}

// New returns a Redactor that preserves the values of the named query parameters
// and redacts all others. With no names, every query value is redacted
// (default-deny).
func New(allowedQueryParams ...string) *Redactor {
	allowed := make(map[string]struct{}, len(allowedQueryParams))
	for _, p := range allowedQueryParams {
		allowed[p] = struct{}{}
	}
	return &Redactor{allowed: allowed}
}

// Default is the default-deny redactor: it redacts every query-param value.
var Default = New()

// URL returns a redacted string form of u: userinfo removed and every
// non-allowlisted query-param value replaced with "REDACTED". Keys, path, and
// fragment are preserved. A nil URL yields "".
func (r *Redactor) URL(u *url.URL) string {
	if u == nil {
		return ""
	}
	c := *u
	c.User = nil
	if c.RawQuery != "" {
		c.RawQuery = r.redactQuery(c.Query())
	}
	return c.String()
}

// redactQuery rebuilds a query string with non-allowlisted values redacted,
// ordered by key for determinism.
func (r *Redactor) redactQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		_, allow := r.allowed[k]
		for _, v := range q[k] {
			if b.Len() > 0 {
				b.WriteByte('&')
			}
			b.WriteString(url.QueryEscape(k))
			b.WriteByte('=')
			if allow {
				b.WriteString(url.QueryEscape(v))
			} else {
				b.WriteString(redactedValue)
			}
		}
	}
	return b.String()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./redact/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add redact/
git commit -m "feat(redact): add default-deny URL redactor"
```

---

## Task 2: `pipeline.Request.SetContext`

**Files:**
- Modify: `pipeline/policy.go`
- Test: `pipeline/policy_test.go` (create if absent; the package's external tests live in `pipeline/pipeline_test.go` as `package pipeline_test` — add a new file in the same package)

- [ ] **Step 1: Write the failing test**

Create `pipeline/context_test.go`:

```go
// pipeline/context_test.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package pipeline_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/dexpace/go-sdk/pipeline"
)

func TestSetContextPropagatesDownstream(t *testing.T) {
	t.Parallel()

	type ctxKey struct{}
	setter := pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		req.SetContext(context.WithValue(req.Raw().Context(), ctxKey{}, "v"))
		return req.Next()
	})
	var got any
	reader := pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		got = req.Raw().Context().Value(ctxKey{})
		return req.Next()
	})

	pl := pipeline.New(transporterFunc(okResponse), setter, reader)
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if got != "v" {
		t.Fatalf("downstream context value = %v, want \"v\"", got)
	}
}

func TestSetContextIgnoresNil(t *testing.T) {
	t.Parallel()

	p := pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		req.SetContext(nil) // must not panic
		return req.Next()
	})
	pl := pipeline.New(transporterFunc(okResponse), p)
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
}
```

(`transporterFunc` and `okResponse` already exist in `pipeline/pipeline_test.go`, same package.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pipeline/ -run TestSetContext -v`
Expected: FAIL — `req.SetContext undefined`.

- [ ] **Step 3: Write the implementation**

In `pipeline/policy.go`, add `"context"` to the import group (stdlib) and add this method after the `Raw` method on `Request`:

```go
// SetContext replaces the underlying request's context. Policies use it to enrich
// the request context — for example, the tracing policy propagates the active
// span so downstream policies and nested spans can find it. A nil context is
// ignored.
func (r *Request) SetContext(ctx context.Context) {
	if ctx == nil {
		return
	}
	r.req = r.req.WithContext(ctx)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pipeline/ -v`
Expected: PASS (the two new tests plus all existing pipeline tests).

- [ ] **Step 5: Commit**

```bash
git add pipeline/policy.go pipeline/context_test.go
git commit -m "feat(pipeline): add Request.SetContext for context enrichment"
```

---

## Task 3: `StageTracing` and `StageMetrics`

**Files:**
- Modify: `pipeline/stage.go`
- Test: `pipeline/stage_test.go`

These two stages go between `StageDate` and `StageLogging`, so the observability
trio sits together just inside the innermost logging stage.

- [ ] **Step 1: Update the failing test**

In `pipeline/stage_test.go`, replace the `ordered` slice in `TestStagesAreOrdered` with:

```go
	ordered := []pipeline.Stage{
		pipeline.StageErrors,
		pipeline.StageClientIdentity,
		pipeline.StageIdempotency,
		pipeline.StageRetry,
		pipeline.StageAuth,
		pipeline.StageDate,
		pipeline.StageTracing,
		pipeline.StageMetrics,
		pipeline.StageLogging,
	}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pipeline/ -run TestStagesAreOrdered -v`
Expected: FAIL — `undefined: pipeline.StageTracing`.

- [ ] **Step 3: Write the implementation**

In `pipeline/stage.go`, replace the `const (...)` block with:

```go
const (
	StageErrors         Stage = iota + 1 // outermost; maps the final result to the typed error model
	StageClientIdentity                  // user-agent and similar identity headers
	StageIdempotency                     // idempotency-key, minted once outside retry
	StageRetry                           // retry pillar; wraps everything below
	StageAuth                            // credential stamping / refresh
	StageDate                            // Date header
	StageTracing                         // span around each attempt
	StageMetrics                         // request metrics around each attempt
	StageLogging                         // innermost; logs the on-the-wire request
)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pipeline/ -v`
Expected: PASS (including `TestNewStagedResolvesOrder`, unaffected).

- [ ] **Step 5: Commit**

```bash
git add pipeline/stage.go pipeline/stage_test.go
git commit -m "feat(pipeline): add StageTracing and StageMetrics anchors"
```

---

## Task 4: tracing SPI and no-op (`instrumentation/tracer.go`)

**Files:**
- Modify: `instrumentation/doc.go`
- Create: `instrumentation/tracer.go`
- Test: `instrumentation/tracer_test.go`

- [ ] **Step 1: Write the failing test**

```go
// instrumentation/tracer_test.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package instrumentation_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dexpace/go-sdk/instrumentation"
)

func TestNoopTracer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	got, span := instrumentation.NoopTracer{}.StartSpan(ctx, "GET")
	if got != ctx {
		t.Fatal("NoopTracer.StartSpan must return the context unchanged")
	}
	if !span.Context().IsZero() {
		t.Fatal("no-op span context must be zero")
	}
	// Must not panic.
	span.SetAttributes(instrumentation.Attr{Key: "k", Value: 1})
	span.RecordError(errors.New("e"))
	span.End()
}

func TestSpanContextIsZero(t *testing.T) {
	t.Parallel()

	var sc instrumentation.SpanContext
	if !sc.IsZero() {
		t.Fatal("zero SpanContext should report IsZero")
	}
	sc.SpanID[0] = 1
	if sc.IsZero() {
		t.Fatal("non-zero SpanContext should not report IsZero")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./instrumentation/ -v`
Expected: FAIL — undefined `NoopTracer` / `SpanContext` / `Attr`.

- [ ] **Step 3: Replace `instrumentation/doc.go`**

```go
// instrumentation/doc.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package instrumentation defines vendor-neutral tracing and metrics seams — a
// Tracer/Span and a Meter with Histogram and UpDownCounter instruments — together
// with no-op defaults and the pipeline policies that drive them. The SDK emits
// spans and metrics through these interfaces without depending on any specific
// observability backend; adapters to OpenTelemetry or similar live in user code.
package instrumentation
```

- [ ] **Step 4: Create `instrumentation/tracer.go`**

```go
// instrumentation/tracer.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package instrumentation

import "context"

// Attr is a key/value attribute attached to a span or metric.
type Attr struct {
	Key   string
	Value any
}

// SpanContext identifies a span for propagation across process boundaries. The
// zero value means "no active span".
type SpanContext struct {
	TraceID [16]byte
	SpanID  [8]byte
	Sampled bool
}

// IsZero reports whether sc carries no trace or span id.
func (sc SpanContext) IsZero() bool {
	return sc.TraceID == [16]byte{} && sc.SpanID == [8]byte{}
}

// Span is a single unit of work within a trace.
type Span interface {
	// SetAttributes attaches key/value attributes to the span.
	SetAttributes(attrs ...Attr)
	// RecordError records err on the span.
	RecordError(err error)
	// End marks the span complete.
	End()
	// Context returns the span's propagation context.
	Context() SpanContext
}

// Tracer starts spans. StartSpan returns a context carrying the new span so that
// spans started from it become children.
type Tracer interface {
	StartSpan(ctx context.Context, name string) (context.Context, Span)
}

// NoopTracer is the default Tracer; it creates spans that do nothing.
type NoopTracer struct{}

// StartSpan returns ctx unchanged and a no-op span.
func (NoopTracer) StartSpan(ctx context.Context, _ string) (context.Context, Span) {
	return ctx, noopSpan{}
}

type noopSpan struct{}

func (noopSpan) SetAttributes(...Attr) {}
func (noopSpan) RecordError(error)     {}
func (noopSpan) End()                  {}
func (noopSpan) Context() SpanContext  { return SpanContext{} }
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./instrumentation/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add instrumentation/doc.go instrumentation/tracer.go instrumentation/tracer_test.go
git commit -m "feat(instrumentation): add Tracer/Span SPI with no-op default"
```

---

## Task 5: metrics SPI and no-op (`instrumentation/meter.go`)

**Files:**
- Create: `instrumentation/meter.go`
- Test: `instrumentation/meter_test.go`

- [ ] **Step 1: Write the failing test**

```go
// instrumentation/meter_test.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package instrumentation_test

import (
	"context"
	"testing"

	"github.com/dexpace/go-sdk/instrumentation"
)

func TestNoopMeter(t *testing.T) {
	t.Parallel()

	m := instrumentation.NoopMeter{}
	// Must not panic and must return usable no-op instruments.
	m.Histogram("h").Record(context.Background(), 1.5, instrumentation.Attr{Key: "k", Value: "v"})
	m.UpDownCounter("c").Add(context.Background(), 1)
	m.UpDownCounter("c").Add(context.Background(), -1)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./instrumentation/ -run TestNoopMeter -v`
Expected: FAIL — undefined `NoopMeter`.

- [ ] **Step 3: Create `instrumentation/meter.go`**

```go
// instrumentation/meter.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package instrumentation

import "context"

// Histogram records a distribution of values (for example request durations in
// seconds).
type Histogram interface {
	Record(ctx context.Context, value float64, attrs ...Attr)
}

// UpDownCounter records an additive value that can rise and fall (for example the
// number of in-flight requests).
type UpDownCounter interface {
	Add(ctx context.Context, delta int64, attrs ...Attr)
}

// Meter creates instruments by name.
type Meter interface {
	Histogram(name string) Histogram
	UpDownCounter(name string) UpDownCounter
}

// NoopMeter is the default Meter; its instruments do nothing.
type NoopMeter struct{}

// Histogram returns a no-op histogram.
func (NoopMeter) Histogram(string) Histogram { return noopHistogram{} }

// UpDownCounter returns a no-op up-down counter.
func (NoopMeter) UpDownCounter(string) UpDownCounter { return noopUpDownCounter{} }

type noopHistogram struct{}

func (noopHistogram) Record(context.Context, float64, ...Attr) {}

type noopUpDownCounter struct{}

func (noopUpDownCounter) Add(context.Context, int64, ...Attr) {}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./instrumentation/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add instrumentation/meter.go instrumentation/meter_test.go
git commit -m "feat(instrumentation): add Meter SPI with no-op default"
```

---

## Task 6: tracing policy (`instrumentation/tracing_policy.go`)

**Files:**
- Create: `instrumentation/tracing_policy.go`
- Test: `instrumentation/tracing_policy_test.go`

- [ ] **Step 1: Write the failing test**

```go
// instrumentation/tracing_policy_test.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package instrumentation_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/dexpace/go-sdk/instrumentation"
	"github.com/dexpace/go-sdk/pipeline"
)

type transporterFunc func(*http.Request) (*http.Response, error)

func (f transporterFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

func okResp(req *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
}

type fakeSpan struct {
	attrs []instrumentation.Attr
	errs  []error
	ended bool
	sc    instrumentation.SpanContext
}

func (s *fakeSpan) SetAttributes(a ...instrumentation.Attr) { s.attrs = append(s.attrs, a...) }
func (s *fakeSpan) RecordError(e error)                     { s.errs = append(s.errs, e) }
func (s *fakeSpan) End()                                    { s.ended = true }
func (s *fakeSpan) Context() instrumentation.SpanContext    { return s.sc }

type fakeTracer struct {
	span    *fakeSpan
	started string
}

func (f *fakeTracer) StartSpan(ctx context.Context, name string) (context.Context, instrumentation.Span) {
	f.started = name
	return ctx, f.span
}

func TestTracingPolicyRecordsSuccess(t *testing.T) {
	t.Parallel()

	span := &fakeSpan{}
	tr := &fakeTracer{span: span}
	pl := pipeline.New(transporterFunc(okResp), instrumentation.NewTracingPolicy(tr, nil))

	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/x?token=secret", nil)
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if tr.started != http.MethodGet {
		t.Fatalf("span name = %q, want GET", tr.started)
	}
	if !span.ended {
		t.Fatal("span not ended")
	}
	if len(span.errs) != 0 {
		t.Fatalf("unexpected RecordError calls: %v", span.errs)
	}
	// url.full attribute must be redacted.
	if !hasAttr(span.attrs, "url.full", "https://api.example.test/x?token=REDACTED") {
		t.Fatalf("url.full not redacted: %v", span.attrs)
	}
	if !hasAttrKey(span.attrs, "http.response.status_code") {
		t.Fatalf("status_code attribute missing: %v", span.attrs)
	}
}

func TestTracingPolicyRecordsError(t *testing.T) {
	t.Parallel()

	span := &fakeSpan{}
	tr := &fakeTracer{span: span}
	boom := errors.New("boom")
	pl := pipeline.New(transporterFunc(func(*http.Request) (*http.Response, error) {
		return nil, boom
	}), instrumentation.NewTracingPolicy(tr, nil))

	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/x", nil)
	_, _ = pl.Do(req)

	if !span.ended {
		t.Fatal("span not ended on error")
	}
	if len(span.errs) != 1 || !errors.Is(span.errs[0], boom) {
		t.Fatalf("RecordError = %v, want boom", span.errs)
	}
}

func TestTracingPolicyInjectsTraceparent(t *testing.T) {
	t.Parallel()

	span := &fakeSpan{sc: instrumentation.SpanContext{
		TraceID: [16]byte{0: 0x0a, 15: 0x0b},
		SpanID:  [8]byte{0: 0x01, 7: 0x02},
		Sampled: true,
	}}
	tr := &fakeTracer{span: span}

	var seen string
	pl := pipeline.New(transporterFunc(func(req *http.Request) (*http.Response, error) {
		seen = req.Header.Get("Traceparent")
		return okResp(req)
	}), instrumentation.NewTracingPolicy(tr, nil))

	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/x", nil)
	resp, _ := pl.Do(req)
	t.Cleanup(func() { _ = resp.Body.Close() })

	want := "00-0a0000000000000000000000000000000b-0100000000000002-01"
	if seen != want {
		t.Fatalf("traceparent = %q, want %q", seen, want)
	}
}

func TestTracingPolicyDoesNotOverrideTraceparent(t *testing.T) {
	t.Parallel()

	span := &fakeSpan{sc: instrumentation.SpanContext{TraceID: [16]byte{0: 1}, SpanID: [8]byte{0: 1}}}
	tr := &fakeTracer{span: span}

	var seen string
	pl := pipeline.New(transporterFunc(func(req *http.Request) (*http.Response, error) {
		seen = req.Header.Get("Traceparent")
		return okResp(req)
	}), instrumentation.NewTracingPolicy(tr, nil))

	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/x", nil)
	req.Header.Set("Traceparent", "caller-value")
	resp, _ := pl.Do(req)
	t.Cleanup(func() { _ = resp.Body.Close() })

	if seen != "caller-value" {
		t.Fatalf("traceparent = %q, want caller-value (not overridden)", seen)
	}
}

func TestTracingPolicyNoTraceparentWhenZero(t *testing.T) {
	t.Parallel()

	span := &fakeSpan{} // zero SpanContext
	tr := &fakeTracer{span: span}

	var seen string
	pl := pipeline.New(transporterFunc(func(req *http.Request) (*http.Response, error) {
		seen = req.Header.Get("Traceparent")
		return okResp(req)
	}), instrumentation.NewTracingPolicy(tr, nil))

	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/x", nil)
	resp, _ := pl.Do(req)
	t.Cleanup(func() { _ = resp.Body.Close() })

	if seen != "" {
		t.Fatalf("traceparent = %q, want empty for zero span context", seen)
	}
}

func hasAttr(attrs []instrumentation.Attr, key string, value any) bool {
	for _, a := range attrs {
		if a.Key == key && a.Value == value {
			return true
		}
	}
	return false
}

func hasAttrKey(attrs []instrumentation.Attr, key string) bool {
	for _, a := range attrs {
		if a.Key == key {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./instrumentation/ -run TestTracingPolicy -v`
Expected: FAIL — `NewTracingPolicy` undefined.

- [ ] **Step 3: Write the implementation**

```go
// instrumentation/tracing_policy.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package instrumentation

import (
	"fmt"
	"net/http"

	"github.com/dexpace/go-sdk/pipeline"
	"github.com/dexpace/go-sdk/redact"
)

// traceparentHeader is the canonical form of the W3C trace-context header.
const traceparentHeader = "Traceparent"

// NewTracingPolicy returns a pipeline policy that records a span around each
// request it wraps, using tracer (defaulting to NoopTracer) and rendering URLs
// with redactor (defaulting to redact.Default). When the span carries a non-zero
// context and the request has no traceparent header, the policy injects a W3C
// traceparent header so downstream services join the trace.
//
// Granularity is placement-determined: inside the retry policy it records a span
// per attempt; outside it, one span per operation.
func NewTracingPolicy(tracer Tracer, redactor *redact.Redactor) pipeline.Policy {
	if tracer == nil {
		tracer = NoopTracer{}
	}
	if redactor == nil {
		redactor = redact.Default
	}
	return pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		raw := req.Raw()
		ctx, span := tracer.StartSpan(raw.Context(), raw.Method)
		req.SetContext(ctx)
		defer span.End()

		span.SetAttributes(
			Attr{Key: "http.request.method", Value: raw.Method},
			Attr{Key: "url.full", Value: redactor.URL(raw.URL)},
			Attr{Key: "server.address", Value: hostOf(raw)},
		)
		injectTraceparent(raw, span.Context())

		resp, err := req.Next()
		if err != nil {
			span.RecordError(err)
			return resp, err
		}
		span.SetAttributes(Attr{Key: "http.response.status_code", Value: resp.StatusCode})
		return resp, nil
	})
}

func hostOf(req *http.Request) string {
	if req.URL == nil {
		return ""
	}
	return req.URL.Host
}

// injectTraceparent sets a W3C traceparent header derived from sc when sc is
// non-zero and the request does not already carry one.
func injectTraceparent(req *http.Request, sc SpanContext) {
	if sc.IsZero() || req.Header.Get(traceparentHeader) != "" {
		return
	}
	flags := "00"
	if sc.Sampled {
		flags = "01"
	}
	req.Header.Set(traceparentHeader, fmt.Sprintf("00-%x-%x-%s", sc.TraceID, sc.SpanID, flags))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./instrumentation/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add instrumentation/tracing_policy.go instrumentation/tracing_policy_test.go
git commit -m "feat(instrumentation): add tracing policy with traceparent injection"
```

---

## Task 7: metrics policy (`instrumentation/metrics_policy.go`)

**Files:**
- Create: `instrumentation/metrics_policy.go`
- Test: `instrumentation/metrics_policy_test.go`

- [ ] **Step 1: Write the failing test**

```go
// instrumentation/metrics_policy_test.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package instrumentation_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/dexpace/go-sdk/instrumentation"
	"github.com/dexpace/go-sdk/pipeline"
)

type recordCall struct {
	value float64
	attrs []instrumentation.Attr
}

type fakeHistogram struct{ calls []recordCall }

func (h *fakeHistogram) Record(_ context.Context, v float64, a ...instrumentation.Attr) {
	h.calls = append(h.calls, recordCall{v, a})
}

type fakeUpDown struct {
	sum   int64
	calls int
}

func (c *fakeUpDown) Add(_ context.Context, d int64, _ ...instrumentation.Attr) {
	c.sum += d
	c.calls++
}

type fakeMeter struct {
	hist *fakeHistogram
	ud   *fakeUpDown
}

func (m *fakeMeter) Histogram(string) instrumentation.Histogram         { return m.hist }
func (m *fakeMeter) UpDownCounter(string) instrumentation.UpDownCounter { return m.ud }

func TestMetricsPolicyRecordsDurationAndBalancesInflight(t *testing.T) {
	t.Parallel()

	m := &fakeMeter{hist: &fakeHistogram{}, ud: &fakeUpDown{}}
	pl := pipeline.New(transporterFunc(okResp), instrumentation.NewMetricsPolicy(m))

	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/x", nil)
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if len(m.hist.calls) != 1 {
		t.Fatalf("histogram calls = %d, want 1", len(m.hist.calls))
	}
	if !hasAttrKey(m.hist.calls[0].attrs, "http.response.status_code") {
		t.Fatalf("duration attrs missing status_code: %v", m.hist.calls[0].attrs)
	}
	if m.ud.calls != 2 || m.ud.sum != 0 {
		t.Fatalf("in-flight gauge calls=%d sum=%d, want 2 and 0 (balanced)", m.ud.calls, m.ud.sum)
	}
}

func TestMetricsPolicyBalancesInflightOnError(t *testing.T) {
	t.Parallel()

	m := &fakeMeter{hist: &fakeHistogram{}, ud: &fakeUpDown{}}
	pl := pipeline.New(transporterFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("boom")
	}), instrumentation.NewMetricsPolicy(m))

	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/x", nil)
	_, _ = pl.Do(req)

	if m.ud.calls != 2 || m.ud.sum != 0 {
		t.Fatalf("in-flight gauge calls=%d sum=%d, want balanced on error", m.ud.calls, m.ud.sum)
	}
	if len(m.hist.calls) != 1 || !hasAttr(m.hist.calls[0].attrs, "error", true) {
		t.Fatalf("duration on error missing error attr: %v", m.hist.calls)
	}
}
```

(`transporterFunc`, `okResp`, `hasAttr`, `hasAttrKey` are defined in `tracing_policy_test.go`, same package.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./instrumentation/ -run TestMetricsPolicy -v`
Expected: FAIL — `NewMetricsPolicy` undefined.

- [ ] **Step 3: Write the implementation**

```go
// instrumentation/metrics_policy.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package instrumentation

import (
	"net/http"
	"time"

	"github.com/dexpace/go-sdk/pipeline"
)

const (
	metricRequestDuration = "http.client.request.duration"
	metricActiveRequests  = "http.client.active_requests"
)

// NewMetricsPolicy returns a pipeline policy that records request metrics using
// meter (defaulting to NoopMeter): a request-duration histogram (seconds) and an
// active-requests up-down counter, each tagged with the request method and, on
// success, the response status code.
//
// Granularity is placement-determined, like the tracing policy.
func NewMetricsPolicy(meter Meter) pipeline.Policy {
	if meter == nil {
		meter = NoopMeter{}
	}
	duration := meter.Histogram(metricRequestDuration)
	active := meter.UpDownCounter(metricActiveRequests)

	return pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		raw := req.Raw()
		ctx := raw.Context()
		method := Attr{Key: "http.request.method", Value: raw.Method}

		active.Add(ctx, 1, method)
		defer active.Add(ctx, -1, method)

		start := time.Now()
		resp, err := req.Next()
		elapsed := time.Since(start).Seconds()

		if err != nil {
			duration.Record(ctx, elapsed, method, Attr{Key: "error", Value: true})
			return resp, err
		}
		duration.Record(ctx, elapsed, method, Attr{Key: "http.response.status_code", Value: resp.StatusCode})
		return resp, nil
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./instrumentation/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add instrumentation/metrics_policy.go instrumentation/metrics_policy_test.go
git commit -m "feat(instrumentation): add metrics policy (duration + in-flight)"
```

---

## Task 8: adopt the shared redactor in `httperr` and `logging`

**Files:**
- Modify: `httperr/httperr.go`, `httperr/transport_error.go`, `logging/logging.go`
- Test: `httperr/httperr_test.go` (add a case), `logging/logging_test.go` (new)

- [ ] **Step 1: Write the failing tests**

Append to `httperr/httperr_test.go` (the `newResponse` helper builds a URL with path `/things`; add a query-redaction case). Add this test:

```go
func TestResponseErrorRedactsQuery(t *testing.T) {
	t.Parallel()

	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(strings.NewReader("bad")),
		Request: &http.Request{
			Method: http.MethodGet,
			URL:    &url.URL{Scheme: "https", Host: "api.example.test", Path: "/things", RawQuery: "api_key=secret"},
		},
	}
	rerr := httperr.FromResponse(resp)
	if rerr == nil {
		t.Fatal("expected a ResponseError")
	}
	if strings.Contains(rerr.URL, "secret") {
		t.Fatalf("URL %q leaked the query secret", rerr.URL)
	}
	if !strings.Contains(rerr.URL, "api_key=REDACTED") {
		t.Fatalf("URL %q should show api_key=REDACTED", rerr.URL)
	}
}
```

Create `logging/logging_test.go`:

```go
// logging/logging_test.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package logging_test

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/dexpace/go-sdk/logging"
	"github.com/dexpace/go-sdk/pipeline"
)

type transporterFunc func(*http.Request) (*http.Response, error)

func (f transporterFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

func TestLoggingRedactsQuerySecret(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	transport := transporterFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
	})
	pl := pipeline.New(transport, logging.NewPolicy(logging.Options{Logger: logger}))

	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/x?token=secret", nil)
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	out := buf.String()
	if strings.Contains(out, "secret") {
		t.Fatalf("log leaked the query secret: %s", out)
	}
	if !strings.Contains(out, "token=REDACTED") {
		t.Fatalf("log should show token=REDACTED: %s", out)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./httperr/ ./logging/ -run 'RedactsQuery|RedactsQuerySecret' -v`
Expected: FAIL — `httperr` still uses `url.Redacted()` (leaves `token=secret`), and `logging.Options` has no redactor / still uses `url.Redacted()`.

- [ ] **Step 3: Update `httperr`**

In `httperr/httperr.go`, add `"github.com/dexpace/go-sdk/redact"` to the import group, and replace the line `rerr.URL = resp.Request.URL.Redacted()` with:

```go
			rerr.URL = redact.Default.URL(resp.Request.URL)
```

In `httperr/transport_error.go`, add `"github.com/dexpace/go-sdk/redact"` to the import group, and replace `te.URL = req.URL.Redacted()` with:

```go
		te.URL = redact.Default.URL(req.URL)
```

- [ ] **Step 4: Update `logging`**

In `logging/logging.go`:
- Replace the `"net/url"` import with `"github.com/dexpace/go-sdk/redact"` (move it to the dexpace import group; `net/url` is no longer needed once the private `redact` func is removed).
- Add a `Redactor` field to `Options`:

```go
type Options struct {
	// Logger is the destination. When nil, slog.Default() is used.
	Logger *slog.Logger
	// Level is the level for request/response records. The zero value
	// (slog.LevelInfo) is used when unset. Failures are always logged at
	// slog.LevelError.
	Level slog.Level
	// Redactor renders URLs for the log records. When nil, redact.Default is used.
	Redactor *redact.Redactor
}
```

- Add a `redactor` field to `Policy` and set it in `NewPolicy`:

```go
type Policy struct {
	logger   *slog.Logger
	level    slog.Level
	redactor *redact.Redactor
}

func NewPolicy(opts Options) *Policy {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	r := opts.Redactor
	if r == nil {
		r = redact.Default
	}
	return &Policy{logger: logger, level: opts.Level, redactor: r}
}
```

- In `Do`, replace `target := redact(raw.URL)` with `target := p.redactor.URL(raw.URL)`.
- Delete the private `func redact(u *url.URL) string { ... }` at the bottom of the file.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./httperr/ ./logging/ -v`
Expected: PASS — including the existing `httperr` tests (a query-less URL redacts to the same string as before).

- [ ] **Step 6: Commit**

```bash
git add httperr/ logging/
git commit -m "refactor: route httperr and logging URL redaction through redact package"
```

---

## Task 9: umbrella options and wiring

**Files:**
- Modify: `options.go`, `client.go`
- Test: `client_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `client_test.go`. Add `"context"` to the stdlib import group and `"github.com/dexpace/go-sdk/instrumentation"` to the dexpace group. (`statusTransport` already exists from the error-model tests.)

```go
type spySpan struct{}

func (spySpan) SetAttributes(...instrumentation.Attr) {}
func (spySpan) RecordError(error)                     {}
func (spySpan) End()                                  {}
func (spySpan) Context() instrumentation.SpanContext  { return instrumentation.SpanContext{} }

type spyTracer struct{ started bool }

func (s *spyTracer) StartSpan(ctx context.Context, _ string) (context.Context, instrumentation.Span) {
	s.started = true
	return ctx, spySpan{}
}

type spyHist struct{ m *spyMeter }

func (h spyHist) Record(context.Context, float64, ...instrumentation.Attr) { h.m.recorded = true }

type spyUD struct{}

func (spyUD) Add(context.Context, int64, ...instrumentation.Attr) {}

type spyMeter struct{ recorded bool }

func (m *spyMeter) Histogram(string) instrumentation.Histogram         { return spyHist{m} }
func (m *spyMeter) UpDownCounter(string) instrumentation.UpDownCounter { return spyUD{} }

func TestWithTracingInstallsPolicy(t *testing.T) {
	t.Parallel()

	tr := &spyTracer{}
	c := dexpace.New(dexpace.WithTransport(statusTransport(http.StatusOK, nil)), dexpace.WithTracing(tr))
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if !tr.started {
		t.Fatal("tracer was not invoked; tracing policy not installed")
	}
}

func TestWithMetricsInstallsPolicy(t *testing.T) {
	t.Parallel()

	m := &spyMeter{}
	c := dexpace.New(dexpace.WithTransport(statusTransport(http.StatusOK, nil)), dexpace.WithMetrics(m))
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if !m.recorded {
		t.Fatal("meter was not invoked; metrics policy not installed")
	}
}

func TestObservabilityOffByDefault(t *testing.T) {
	t.Parallel()

	// A client with neither WithTracing nor WithMetrics must still work; the spy
	// types are not passed, so there is nothing to assert beyond a clean Do.
	c := dexpace.New(dexpace.WithTransport(statusTransport(http.StatusOK, nil)))
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test . -run 'TestWithTracingInstallsPolicy|TestWithMetricsInstallsPolicy' -v`
Expected: FAIL — `dexpace.WithTracing` / `dexpace.WithMetrics` undefined.

- [ ] **Step 3: Add options in `options.go`**

Add `"github.com/dexpace/go-sdk/instrumentation"` to the dexpace import group. Add fields to `config` (after `errorsEnabled bool`):

```go
	tracer      instrumentation.Tracer
	meter       instrumentation.Meter
	redactAllow []string
```

Add the options (after `WithErrors`):

```go
// WithTracing installs a tracing policy that records a span around each request
// attempt using tracer, injecting a W3C traceparent header. A nil tracer is
// ignored (no policy installed). Off by default.
func WithTracing(tracer instrumentation.Tracer) Option {
	return func(c *config) { c.tracer = tracer }
}

// WithMetrics installs a metrics policy that records request duration and
// in-flight count using meter. A nil meter is ignored (no policy installed). Off
// by default.
func WithMetrics(meter instrumentation.Meter) Option {
	return func(c *config) { c.meter = meter }
}

// WithRedactionAllowlist preserves the values of the named query parameters in
// redacted URLs (logs, traces). All other query values are redacted by default.
// Applies to the logging, tracing, and metrics policies.
func WithRedactionAllowlist(params ...string) Option {
	return func(c *config) { c.redactAllow = params }
}
```

- [ ] **Step 4: Wire it in `client.go`**

Add `"github.com/dexpace/go-sdk/instrumentation"` and `"github.com/dexpace/go-sdk/redact"` to the dexpace import group. In `New`, build the shared redactor right after the `placements := []pipeline.Placement{...}` literal and the errors placement, then wire logging/tracing/metrics. Specifically:

Build the redactor (place it near the top of `New`, after options are applied):

```go
	redactor := redact.Default
	if len(cfg.redactAllow) > 0 {
		redactor = redact.New(cfg.redactAllow...)
	}
```

Change the logging placement to pass the redactor:

```go
	if cfg.logging {
		placements = append(placements,
			pipeline.At(pipeline.StageLogging, logging.NewPolicy(logging.Options{Logger: cfg.logger, Redactor: redactor})))
	}
```

Add the tracing and metrics placements (next to the logging one):

```go
	if cfg.tracer != nil {
		placements = append(placements,
			pipeline.At(pipeline.StageTracing, instrumentation.NewTracingPolicy(cfg.tracer, redactor)))
	}
	if cfg.meter != nil {
		placements = append(placements,
			pipeline.At(pipeline.StageMetrics, instrumentation.NewMetricsPolicy(cfg.meter)))
	}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test . -v`
Expected: PASS (the three new tests plus all pre-existing umbrella tests).

- [ ] **Step 6: Run the full suite**

Run: `go test ./...`
Expected: PASS across every package.

- [ ] **Step 7: Commit**

```bash
git add client.go options.go client_test.go
git commit -m "feat: add WithTracing, WithMetrics, and WithRedactionAllowlist"
```

---

## Task 10: docs and full gate

**Files:**
- Modify: `doc.go`, `README.md`

- [ ] **Step 1: Mention tracing/metrics in `doc.go`**

Read `doc.go`. Within the existing `package dexpace` doc comment (a single contiguous `//` block above `package dexpace`; do not add a second package clause or duplicate the license header), add this sentence at a natural spot describing behaviour/options:

```go
// Tracing and metrics are opt-in: WithTracing and WithMetrics install policies
// that emit spans and request metrics through the instrumentation package's
// vendor-neutral interfaces (no-op by default). WithRedactionAllowlist controls
// which query-param values survive redaction in logs and traces.
```

- [ ] **Step 2: Mention the new options in `README.md`**

Read `README.md`. In its options/usage section (which already lists `WithErrors` and friends), add short factual entries for `WithTracing(tracer)`, `WithMetrics(meter)`, and `WithRedactionAllowlist(params...)` — opt-in observability via vendor-neutral SPIs, no-op by default; note URLs are redacted (userinfo + query values) by default. Match the surrounding style; keep the edit tight.

- [ ] **Step 3: Run the full gate**

Run:
```bash
gofmt -l .
go vet ./...
go test -race ./...
```
Expected: `gofmt -l .` prints nothing; `go vet` clean; every package passes under the race detector (placeholder packages show `[no test files]` — by now `instrumentation` and `redact` have tests).

- [ ] **Step 4: Commit**

```bash
git add doc.go README.md
git commit -m "docs: document opt-in tracing, metrics, and redaction options"
```

---

## Self-Review notes (for the implementer)

- **Spec coverage:** redactor (Task 1, adopted in Task 8); `SetContext` (Task 2); stages (Task 3); tracing SPI/no-op (Task 4); metrics SPI/no-op (Task 5); tracing policy + traceparent (Task 6); metrics policy (Task 7); umbrella opt-in wiring (Task 9); docs (Task 10). All spec sections map to a task.
- **Type consistency:** `Attr{Key, Value}`, `SpanContext{TraceID, SpanID, Sampled}`, `Tracer.StartSpan(ctx, name) (ctx, Span)`, `Span{SetAttributes, RecordError, End, Context}`, `Meter.Histogram/UpDownCounter`, `Histogram.Record(ctx, float64, ...Attr)`, `UpDownCounter.Add(ctx, int64, ...Attr)`, `redact.New`/`redact.Default`/`Redactor.URL`, `NewTracingPolicy(Tracer, *redact.Redactor)`, `NewMetricsPolicy(Meter)` — used identically across tasks.
- **Defaults guard:** `TestObservabilityOffByDefault`; policies installed only when tracer/meter non-nil.
- **Behaviour change:** logging and httperr now redact query values (default-deny), not just userinfo — a security improvement; note in release notes.
- **`make check`** green before opening the PR.
