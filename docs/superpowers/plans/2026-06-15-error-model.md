# Error Model Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an opt-in typed error model — `httperr.TransportError` wrapping plus non-2xx `ResponseError` mapping — activated by a single `dexpace.WithErrors()` option and installed as the outermost pipeline stage so retry keeps operating on raw responses.

**Architecture:** A new outermost `pipeline.StageErrors` hosts one inline `errorsPolicy` (in the umbrella package) that maps the *final* result of the chain: transport errors become `*httperr.TransportError` (context errors pass through unwrapped), and non-2xx responses become `*httperr.ResponseError`. Because the mapper wraps retry, retry still sees raw responses/errors during its attempts; because `TransportError.Unwrap` is lossless, retry's existing classification is unaffected. Defaults stay pure net/http — the model is off unless `WithErrors()` is passed.

**Tech Stack:** Go 1.26+, standard library only (`net`, `net/http`, `errors`, `context`, `fmt`). Zero third-party runtime dependencies.

**Conventions every task must follow:**
- MIT license header on every `.go` file (src and tests), before the `package` clause:
  ```go
  // Copyright (c) 2026 dexpace and Omar Aljarrah.
  // Licensed under the MIT License. See LICENSE in the repository root for details.
  ```
- Import groups: stdlib, blank line, then `github.com/dexpace/go-sdk/...` (alphabetized).
- Tests use `t.Parallel()`; response bodies closed via `t.Cleanup`. Local `transporterFunc` fakes, no mocking frameworks.
- Tools: Go 1.26.3 is installed; `gofumpt`/`goimports`/`golangci-lint` are NOT installed locally — use `gofmt`, `go vet`, and `go test -race`; CI runs the linters.
- Run commands from the repo root `/Users/omar/dexpace/go-sdk`.

---

## File Structure

| Path | Responsibility |
|---|---|
| `pipeline/stage.go` (modify) | add `StageErrors` as the new outermost stage; update the `Stage` doc |
| `pipeline/stage_test.go` (modify) | extend the ordering test to include `StageErrors` |
| `httperr/transport_error.go` (new) | `TransportError` type + `FromError` helper |
| `httperr/transport_error_test.go` (new) | unit tests for both |
| `options.go` (modify) | `errorsEnabled bool` field + `WithErrors()` option |
| `client.go` (modify) | inline `errorsPolicy()`; wire `At(StageErrors, …)` when enabled |
| `client_test.go` (modify) | activation + behaviour + retry-interaction tests |
| `README.md`, `doc.go` (modify) | mention `WithErrors` |

---

## Task 1: Add the outermost `StageErrors`

**Files:**
- Modify: `pipeline/stage.go`
- Test: `pipeline/stage_test.go`

`StageErrors` becomes the new outermost stage (smallest ordinal). The other stage
values shift up by one; that is fine because all ordering is relational and the
sort key (`int(stage)*4 + offset`) still has no collisions.

- [ ] **Step 1: Update the failing test first**

In `pipeline/stage_test.go`, the existing `TestStagesAreOrdered` lists six stages.
Replace its `ordered` slice literal so `StageErrors` is first:

```go
	ordered := []pipeline.Stage{
		pipeline.StageErrors,
		pipeline.StageClientIdentity,
		pipeline.StageIdempotency,
		pipeline.StageRetry,
		pipeline.StageAuth,
		pipeline.StageDate,
		pipeline.StageLogging,
	}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pipeline/ -run TestStagesAreOrdered -v`
Expected: FAIL — `undefined: pipeline.StageErrors`.

- [ ] **Step 3: Implement**

In `pipeline/stage.go`, replace the `const (...)` block with (note `StageErrors`
is now first, so `iota + 1` assigns it 1 and the rest shift up):

```go
const (
	StageErrors         Stage = iota + 1 // outermost; maps the final result to the typed error model
	StageClientIdentity                  // user-agent and similar identity headers
	StageIdempotency                     // idempotency-key, minted once outside retry
	StageRetry                           // retry pillar; wraps everything below
	StageAuth                            // credential stamping / refresh
	StageDate                            // Date header
	StageLogging                         // innermost; logs the on-the-wire request
)
```

Also update the `Stage` type doc comment (just above `type Stage int`) so the
outermost stage is named correctly — replace the parenthetical
"(StageClientIdentity)" with "(StageErrors)":

```go
// Stage names an anchor point in the standard policy order, from outermost
// (StageErrors) to innermost (StageLogging). Stages are used at assembly time to
// place policies deterministically; the running pipeline is still a flat ordered
// list. Use [At], [Before], and [After] to position a [Policy] relative to a
// Stage, then build with [NewStaged].
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pipeline/ -v`
Expected: PASS — all pipeline tests, including `TestStagesAreOrdered` and `TestNewStagedResolvesOrder`.

- [ ] **Step 5: Commit**

```bash
git add pipeline/stage.go pipeline/stage_test.go
git commit -m "feat(pipeline): add outermost StageErrors anchor"
```

---

## Task 2: `httperr.TransportError` and `FromError`

**Files:**
- Create: `httperr/transport_error.go`
- Test: `httperr/transport_error_test.go`

- [ ] **Step 1: Write the failing test**

```go
// httperr/transport_error_test.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package httperr_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/dexpace/go-sdk/httperr"
)

func newReq() *http.Request {
	return &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Scheme: "https", Host: "api.example.test", Path: "/things"},
	}
}

// timeoutErr is a net.Error reporting a timeout.
type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return false }

func TestFromErrorNil(t *testing.T) {
	t.Parallel()

	if got := httperr.FromError(nil, newReq()); got != nil {
		t.Fatalf("FromError(nil) = %v, want nil", got)
	}
}

func TestFromErrorPassesThroughContextErrors(t *testing.T) {
	t.Parallel()

	// Even wrapped, a context error must pass through unwrapped.
	wrapped := &url.Error{Op: "Get", URL: "https://x", Err: context.Canceled}
	got := httperr.FromError(wrapped, newReq())

	var te *httperr.TransportError
	if errors.As(got, &te) {
		t.Fatal("context error must NOT be wrapped as *TransportError")
	}
	if !errors.Is(got, context.Canceled) {
		t.Fatalf("FromError lost context.Canceled: %v", got)
	}
}

func TestFromErrorWrapsTransportFailure(t *testing.T) {
	t.Parallel()

	cause := errors.New("dial tcp: connection refused")
	got := httperr.FromError(cause, newReq())

	var te *httperr.TransportError
	if !errors.As(got, &te) {
		t.Fatalf("FromError = %T, want *TransportError", got)
	}
	if te.Method != http.MethodGet {
		t.Fatalf("Method = %q, want GET", te.Method)
	}
	if te.URL != "https://api.example.test/things" {
		t.Fatalf("URL = %q, want the redacted request URL", te.URL)
	}
	if !errors.Is(got, cause) {
		t.Fatal("Unwrap must reach the underlying cause")
	}
}

func TestTransportErrorTimeout(t *testing.T) {
	t.Parallel()

	timeout := httperr.FromError(timeoutErr{}, newReq())
	var te *httperr.TransportError
	if !errors.As(timeout, &te) || !te.Timeout() {
		t.Fatal("Timeout() should be true for a net.Error timeout cause")
	}

	plain := httperr.FromError(errors.New("nope"), newReq())
	var te2 *httperr.TransportError
	if !errors.As(plain, &te2) || te2.Timeout() {
		t.Fatal("Timeout() should be false for a non-net cause")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./httperr/ -run 'FromError|TransportError' -v`
Expected: FAIL — `undefined: httperr.FromError` / `httperr.TransportError`.

- [ ] **Step 3: Write minimal implementation**

```go
// httperr/transport_error.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package httperr

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
)

// TransportError reports that a request never produced a response — for example
// a DNS failure, a refused connection, a TLS error, or a network timeout. It
// wraps the underlying net/http cause; Unwrap preserves errors.Is/As to that
// cause (including *url.Error and net.Error).
//
// It is returned only when the typed error model is enabled (dexpace.WithErrors).
type TransportError struct {
	// Method is the request method that failed.
	Method string
	// URL is the redacted request URL.
	URL string
	// Err is the underlying cause.
	Err error
}

// Error implements error.
func (e *TransportError) Error() string {
	if e.URL == "" {
		return fmt.Sprintf("transport error: %v", e.Err)
	}
	return fmt.Sprintf("transport error: %s %s: %v", e.Method, e.URL, e.Err)
}

// Unwrap returns the underlying cause so errors.Is/As reach through it.
func (e *TransportError) Unwrap() error { return e.Err }

// Timeout reports whether the underlying cause is a network timeout. It mirrors
// net.Error.Timeout(); it is false for non-net causes.
func (e *TransportError) Timeout() bool {
	var ne net.Error
	return errors.As(e.Err, &ne) && ne.Timeout()
}

// FromError maps a transport-level error to the typed model. It returns nil for
// a nil error and returns context cancellation/deadline errors unchanged (they
// are the caller's deadline, not a transport fault). Any other error is wrapped
// in a [TransportError] carrying req's method and redacted URL.
func FromError(err error, req *http.Request) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	te := &TransportError{Err: err}
	if req != nil {
		te.Method = req.Method
		if req.URL != nil {
			te.URL = req.URL.Redacted()
		}
	}
	return te
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./httperr/ -v`
Expected: PASS — the new tests plus the existing `ResponseError` tests.

- [ ] **Step 5: Commit**

```bash
git add httperr/transport_error.go httperr/transport_error_test.go
git commit -m "feat(httperr): add TransportError and FromError"
```

---

## Task 3: `WithErrors()` option and the inline error policy

**Files:**
- Modify: `options.go`, `client.go`
- Test: `client_test.go`

The policy is the outermost stage; it maps the final result. Retry (inner) keeps
seeing raw responses.

- [ ] **Step 1: Write the failing tests**

Append to `client_test.go`. It already defines `transporterFunc`, `captureTransport`,
and imports `net/http`, `testing`, `strings`, `dexpace`, and `pipeline`. Add
`"errors"` to the stdlib group and `"github.com/dexpace/go-sdk/httperr"` and
`"github.com/dexpace/go-sdk/retry"` to the dexpace group (retry may already be absent — add it).

```go
// statusTransport returns a fresh response with the given status code each call.
func statusTransport(code int, calls *int) transporterFunc {
	return func(req *http.Request) (*http.Response, error) {
		if calls != nil {
			*calls++
		}
		return &http.Response{
			StatusCode: code,
			Status:     http.StatusText(code),
			Body:       http.NoBody,
			Request:    req,
		}, nil
	}
}

// errTransport always fails with a transport error.
func errTransport(err error) transporterFunc {
	return func(*http.Request) (*http.Response, error) { return nil, err }
}

func TestErrorsOffByDefault(t *testing.T) {
	t.Parallel()

	// 404 is a successful round-trip by default.
	c := dexpace.New(dexpace.WithTransport(statusTransport(http.StatusNotFound, nil)))
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do = %v, want nil error by default", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}

	// Transport error is raw, not a *TransportError.
	cause := errors.New("connection refused")
	c2 := dexpace.New(
		dexpace.WithTransport(errTransport(cause)),
		dexpace.WithRetry(retry.Options{MaxRetries: -1}),
	)
	_, err2 := c2.Do(req)
	var te *httperr.TransportError
	if errors.As(err2, &te) {
		t.Fatal("transport error should be raw without WithErrors()")
	}
	if !errors.Is(err2, cause) {
		t.Fatalf("err = %v, want the raw cause", err2)
	}
}

func TestWithErrorsConvertsStatusError(t *testing.T) {
	t.Parallel()

	c := dexpace.New(
		dexpace.WithTransport(statusTransport(http.StatusNotFound, nil)),
		dexpace.WithErrors(),
	)
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := c.Do(req)
	if resp != nil {
		t.Cleanup(func() { _ = resp.Body.Close() })
	}

	var rerr *httperr.ResponseError
	if !errors.As(err, &rerr) {
		t.Fatalf("err = %T, want *httperr.ResponseError", err)
	}
	if rerr.StatusCode != http.StatusNotFound {
		t.Fatalf("StatusCode = %d, want 404", rerr.StatusCode)
	}
}

func TestWithErrorsWrapsTransportError(t *testing.T) {
	t.Parallel()

	cause := errors.New("dial tcp: connection refused")
	c := dexpace.New(
		dexpace.WithTransport(errTransport(cause)),
		dexpace.WithErrors(),
		dexpace.WithRetry(retry.Options{MaxRetries: -1}), // no retries: fail fast
	)
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	_, err := c.Do(req)

	var te *httperr.TransportError
	if !errors.As(err, &te) {
		t.Fatalf("err = %T, want *httperr.TransportError", err)
	}
	if !errors.Is(err, cause) {
		t.Fatal("Unwrap must reach the underlying cause")
	}
}

func TestWithErrorsRetryStillSeesRawResponse(t *testing.T) {
	t.Parallel()

	var calls int
	c := dexpace.New(
		dexpace.WithTransport(statusTransport(http.StatusServiceUnavailable, &calls)),
		dexpace.WithErrors(),
		dexpace.WithRetry(retry.Options{MaxRetries: 2, BaseDelay: 1, MaxDelay: 1}),
	)
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := c.Do(req)
	if resp != nil {
		t.Cleanup(func() { _ = resp.Body.Close() })
	}

	if calls != 3 { // 503 is retryable: initial + 2 retries
		t.Fatalf("transport calls = %d, want 3 (retry saw raw 503s)", calls)
	}
	var rerr *httperr.ResponseError
	if !errors.As(err, &rerr) || rerr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("final err = %v, want *ResponseError with 503", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test . -run 'TestErrorsOffByDefault|TestWithErrors' -v`
Expected: FAIL — `undefined: dexpace.WithErrors`.

- [ ] **Step 3: Add the option in `options.go`**

Add a field to the `config` struct (after the `after []pipeline.Placement` field):

```go
	errorsEnabled bool
```

Add the option (place it after `WithDate`):

```go
// WithErrors enables the typed error model. With it, Client.Do returns a
// *httperr.ResponseError for a non-2xx response and a *httperr.TransportError
// for a request that never produced a response (context cancellation/deadline
// errors are returned unchanged). Off by default: without it, Do mirrors
// http.Client.Do — a non-2xx status is not an error, and transport failures
// surface as raw net/http errors.
func WithErrors() Option {
	return func(c *config) { c.errorsEnabled = true }
}
```

- [ ] **Step 4: Add the policy and wiring in `client.go`**

Add `"github.com/dexpace/go-sdk/httperr"` to the dexpace import group
(alphabetical: auth, header, httperr, idempotency, logging, pipeline, retry, transport).

Add the inline policy near `datePolicy`:

```go
// errorsPolicy maps the final result of the chain to the typed error model: a
// transport failure becomes a *httperr.TransportError (context errors pass
// through unchanged), and a non-2xx response becomes a *httperr.ResponseError.
// It is the outermost policy, so retry still operates on raw responses.
func errorsPolicy() pipeline.Policy {
	return pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		resp, err := req.Next()
		if err != nil {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			return nil, httperr.FromError(err, req.Raw())
		}
		if rerr := httperr.FromResponse(resp); rerr != nil {
			return resp, rerr
		}
		return resp, nil
	})
}
```

In `New`, add the placement (put it with the other conditional placements, e.g.
right after the `placements := []pipeline.Placement{...}` literal):

```go
	if cfg.errorsEnabled {
		placements = append(placements, pipeline.At(pipeline.StageErrors, errorsPolicy()))
	}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test . -v`
Expected: PASS — the four new tests plus all pre-existing umbrella tests.

- [ ] **Step 6: Run the full suite**

Run: `go test ./...`
Expected: PASS across every package.

- [ ] **Step 7: Commit**

```bash
git add client.go options.go client_test.go
git commit -m "feat: add opt-in typed error model via WithErrors"
```

---

## Task 4: Docs and full gate

**Files:**
- Modify: `README.md`, `doc.go`
- Modify: `pipeline/doc.go` is NOT required (its stage-ordering prose is generic).

- [ ] **Step 1: Mention `WithErrors` in the umbrella package doc**

Read `doc.go`. In its options/behaviour description, add a sentence noting the
opt-in error model. If `doc.go` lists options or behaviours, add:

```go
// By default Client.Do mirrors http.Client.Do: a non-2xx status is not an error.
// Enable the typed error model with WithErrors to receive *httperr.ResponseError
// for non-2xx responses and *httperr.TransportError for transport failures.
```
Place it as a complete sentence within the existing package comment (do not add a
second package clause or duplicate the license header).

- [ ] **Step 2: Mention `WithErrors` in README.md**

Read `README.md`. Find the options/usage section and add a short bullet or
sentence: that `WithErrors()` opts into typed errors (`*httperr.ResponseError`
for non-2xx, `*httperr.TransportError` for transport failures), off by default to
preserve net/http semantics. Keep the edit tight; do not restructure the README.

- [ ] **Step 3: Run the full gate**

Run:
```bash
gofmt -l .
go vet ./...
go test -race ./...
```
Expected: `gofmt -l .` prints nothing; `go vet` clean; every package passes under
the race detector (placeholder packages show `[no test files]`).

- [ ] **Step 4: Commit**

```bash
git add README.md doc.go
git commit -m "docs: document the opt-in WithErrors error model"
```

---

## Self-Review notes (for the implementer)

- **Spec coverage:** opt-in activation (Task 3 `WithErrors`); outermost placement so
  retry sees raw responses (Task 1 `StageErrors` + Task 3 wiring + the
  `TestWithErrorsRetryStillSeesRawResponse` test); `TransportError` + `FromError`
  with lossless `Unwrap`, `Timeout()`, and context pass-through (Task 2); non-2xx →
  `ResponseError` returning `(resp, rerr)` (Task 3 policy). All spec sections map to
  a task.
- **Behaviour-change guard:** `TestErrorsOffByDefault` proves defaults are unchanged.
- **Type consistency:** `FromError(err error, req *http.Request) error` and
  `TransportError{Method, URL, Err}` are used identically in Tasks 2 and 3;
  `StageErrors` from Task 1 is referenced in Task 3.
- **Naming note:** the config field is `errorsEnabled` (not `errors`) to avoid any
  confusion with the `errors` package at call sites — a deliberate clarity choice.
- **`make check`** should be green before opening/After updating the PR.
