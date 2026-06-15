# Pipeline Completeness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring the Go SDK pipeline to enterprise parity with the Java/Python SDKs — a `Stage` ordering model, default-on idempotency keys, method-aware retry, opt-in set-date, and net/http-owned redirect configuration.

**Architecture:** A new assembly-time `Stage` layer in `pipeline` resolves stage-anchored placements into the existing flat `[]Policy` slice (the engine is unchanged). New policies live in their own packages where they carry real logic (`idempotency`) or inline in the umbrella where trivial (set-date, user-agent). `retry` and `transport` gain small enhancements; `dexpace.New` is rewired to assemble built-ins as placements.

**Tech Stack:** Go 1.26+, standard library only (`net/http`, `crypto/rand`, `log/slog`, `iter`). Zero third-party runtime dependencies. `gofumpt`/`goimports`/`go vet`/`golangci-lint` clean. Table-driven parallel tests with local `transporterFunc` fakes.

**Conventions every task must follow:**
- MIT license header on every `.go` file (src and tests), before the `package` clause:
  ```go
  // Copyright (c) 2026 dexpace and Omar Aljarrah.
  // Licensed under the MIT License. See LICENSE in the repository root for details.
  ```
- Import groups: stdlib, then `github.com/dexpace/go-sdk/...`.
- Tests use `t.Parallel()`; response bodies closed via `t.Cleanup`.
- Run `go test ./...` from the repo root `/Users/omar/dexpace/go-sdk`.

---

## File Structure

| Path | Responsibility |
|---|---|
| `pipeline/stage.go` (new) | `Stage` type + constants, `Placement`, `At`/`Before`/`After`, `NewStaged`, resolver |
| `pipeline/idempotent.go` (new) | `MarkIdempotent`/`IsIdempotent` cross-policy coordination |
| `pipeline/stage_test.go` (new) | resolver ordering tests |
| `pipeline/idempotent_test.go` (new) | mark/read tests |
| `pipeline/doc.go` (modify) | document the stage model |
| `idempotency/doc.go`, `idempotency/policy.go` (new) | idempotency-key policy |
| `idempotency/policy_test.go` (new) | policy tests |
| `retry/retry.go` (modify) | method-aware transport-error retry |
| `retry/retry_test.go` (modify/new) | method-aware retry tests |
| `transport/transport.go` (modify) | `WithMaxRedirects`, `WithRedirectPolicy` |
| `transport/transport_test.go` (modify/new) | redirect-config tests |
| `client.go`, `options.go` (modify) | stage-based assembly; new options |
| `client_test.go` (new/modify) | assembly + option behavior tests |

---

## Task 1: `pipeline.Stage` type and constants

**Files:**
- Create: `pipeline/stage.go`
- Test: `pipeline/stage_test.go`

- [ ] **Step 1: Write the failing test**

```go
// pipeline/stage_test.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package pipeline_test

import (
	"testing"

	"github.com/dexpace/go-sdk/pipeline"
)

func TestStagesAreOrdered(t *testing.T) {
	t.Parallel()

	ordered := []pipeline.Stage{
		pipeline.StageClientIdentity,
		pipeline.StageIdempotency,
		pipeline.StageRetry,
		pipeline.StageAuth,
		pipeline.StageDate,
		pipeline.StageLogging,
	}
	for i := 1; i < len(ordered); i++ {
		if !(ordered[i-1] < ordered[i]) {
			t.Fatalf("stage %d not less than stage %d", ordered[i-1], ordered[i])
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pipeline/ -run TestStagesAreOrdered -v`
Expected: FAIL — `undefined: pipeline.Stage` / `pipeline.StageClientIdentity`.

- [ ] **Step 3: Write minimal implementation**

```go
// pipeline/stage.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package pipeline

// Stage names an anchor point in the standard policy order, from outermost
// (StageClientIdentity) to innermost (StageLogging). Stages are used at assembly
// time to place policies deterministically; the running pipeline is still a flat
// ordered list. Use [At], [Before], and [After] to position a [Policy] relative
// to a Stage, then build with [NewStaged].
type Stage int

// The standard stages, in execution order. An earlier stage wraps the later
// ones: retry (StageRetry) is outside auth (StageAuth), so a 401-triggered token
// refresh re-runs per attempt; logging (StageLogging) is innermost, so it records
// the request as actually sent.
const (
	StageClientIdentity Stage = iota + 1 // user-agent and similar identity headers
	StageIdempotency                     // idempotency-key, minted once outside retry
	StageRetry                           // retry pillar; wraps everything below
	StageAuth                            // credential stamping / refresh
	StageDate                            // Date header
	StageLogging                         // innermost; logs the on-the-wire request
)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pipeline/ -run TestStagesAreOrdered -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pipeline/stage.go pipeline/stage_test.go
git commit -m "feat(pipeline): add Stage type and ordered stage constants"
```

---

## Task 2: `Placement` and `At`/`Before`/`After`

**Files:**
- Modify: `pipeline/stage.go`
- Test: `pipeline/stage_test.go`

- [ ] **Step 1: Write the failing test**

```go
// append to pipeline/stage_test.go
func TestPlacementConstructors(t *testing.T) {
	t.Parallel()

	p := pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		return req.Next()
	})
	// Must compile and return non-zero placements; ordering is covered in
	// TestNewStagedResolvesOrder.
	_ = pipeline.At(pipeline.StageRetry, p)
	_ = pipeline.Before(pipeline.StageRetry, p)
	_ = pipeline.After(pipeline.StageAuth, p)
}
```

Add `"net/http"` to the test file's imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pipeline/ -run TestPlacementConstructors -v`
Expected: FAIL — `undefined: pipeline.At`.

- [ ] **Step 3: Write minimal implementation**

```go
// append to pipeline/stage.go

// Placement pairs a [Policy] with where it belongs in stage order. Construct one
// with [At], [Before], or [After] and pass it to [NewStaged].
type Placement struct {
	stage  Stage
	offset int8 // -1 before, 0 at (pillar), +1 after
	policy Policy
}

// At places p exactly at stage s (a "pillar"). When two placements target the
// same stage with the same offset, insertion order is preserved; supplying At
// for a stage already occupied therefore appends after the earlier one.
func At(s Stage, p Policy) Placement { return Placement{stage: s, offset: 0, policy: p} }

// Before places p immediately outside (before) stage s.
func Before(s Stage, p Policy) Placement { return Placement{stage: s, offset: -1, policy: p} }

// After places p immediately inside (after) stage s.
func After(s Stage, p Policy) Placement { return Placement{stage: s, offset: 1, policy: p} }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pipeline/ -run TestPlacementConstructors -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pipeline/stage.go pipeline/stage_test.go
git commit -m "feat(pipeline): add Placement and At/Before/After constructors"
```

---

## Task 3: `NewStaged` resolver

**Files:**
- Modify: `pipeline/stage.go`
- Test: `pipeline/stage_test.go`

The resolver computes a sort key `int(stage)*4 + offset*1` for each placement,
then **stable-sorts** so ties preserve insertion order, then builds the pipeline
with the existing `New`. (Multiplying the stage by 4 leaves room for the
`-1/0/+1` offsets without collisions between adjacent stages.)

- [ ] **Step 1: Write the failing test**

```go
// append to pipeline/stage_test.go
func TestNewStagedResolvesOrder(t *testing.T) {
	t.Parallel()

	var order []string
	mark := func(name string) pipeline.Policy {
		return pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
			order = append(order, name)
			return req.Next()
		})
	}
	tr := transporterFunc(func(req *http.Request) (*http.Response, error) {
		order = append(order, "transport")
		return okResponse(req)
	})

	// Deliberately shuffled input; resolver must sort by stage then offset,
	// preserving insertion order within the same (stage, offset).
	pl := pipeline.NewStaged(tr,
		pipeline.At(pipeline.StageAuth, mark("auth")),
		pipeline.At(pipeline.StageClientIdentity, mark("ua")),
		pipeline.Before(pipeline.StageRetry, mark("before-retry")),
		pipeline.At(pipeline.StageRetry, mark("retry")),
		pipeline.After(pipeline.StageRetry, mark("after-retry")),
		pipeline.At(pipeline.StageRetry, mark("retry2")), // same stage+offset → after retry
		pipeline.At(pipeline.StageLogging, mark("log")),
	)

	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	want := []string{"ua", "before-retry", "retry", "retry2", "after-retry", "auth", "log", "transport"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", order, want)
	}
}
```

Add `"strings"` to the test file's imports if not already present.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pipeline/ -run TestNewStagedResolvesOrder -v`
Expected: FAIL — `undefined: pipeline.NewStaged`.

- [ ] **Step 3: Write minimal implementation**

```go
// append to pipeline/stage.go (add "sort" to the file's imports)

// NewStaged resolves placements into a deterministic order and builds a
// [Pipeline]. Placements are sorted by stage, then by offset (before, at,
// after); placements sharing the same stage and offset keep the order in which
// they were supplied. transport must be non-nil; passing nil panics, matching
// [New].
func NewStaged(transport Transporter, placements ...Placement) Pipeline {
	sorted := make([]Placement, len(placements))
	copy(sorted, placements)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sortKey(sorted[i]) < sortKey(sorted[j])
	})
	policies := make([]Policy, len(sorted))
	for i, pl := range sorted {
		policies[i] = pl.policy
	}
	return New(transport, policies...)
}

func sortKey(p Placement) int { return int(p.stage)*4 + int(p.offset) }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pipeline/ -run TestNewStagedResolvesOrder -v`
Expected: PASS.

- [ ] **Step 5: Run the full pipeline package to confirm no regressions**

Run: `go test ./pipeline/ -v`
Expected: PASS (all existing tests plus the new ones).

- [ ] **Step 6: Commit**

```bash
git add pipeline/stage.go pipeline/stage_test.go
git commit -m "feat(pipeline): resolve stage placements into an ordered pipeline"
```

---

## Task 4: Idempotency coordination markers in `pipeline`

**Files:**
- Create: `pipeline/idempotent.go`
- Test: `pipeline/idempotent_test.go`

This is how the idempotency and retry policies cooperate without an import cycle
(retry would otherwise need to import idempotency). The marker is stored as a
request value under an unexported key.

- [ ] **Step 1: Write the failing test**

```go
// pipeline/idempotent_test.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package pipeline_test

import (
	"net/http"
	"testing"

	"github.com/dexpace/go-sdk/pipeline"
)

func TestIdempotentMarker(t *testing.T) {
	t.Parallel()

	var beforeMark, afterMark bool
	marker := pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		beforeMark = pipeline.IsIdempotent(req)
		pipeline.MarkIdempotent(req)
		return req.Next()
	})
	reader := pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		afterMark = pipeline.IsIdempotent(req)
		return req.Next()
	})

	pl := pipeline.New(transporterFunc(okResponse), marker, reader)
	req, _ := http.NewRequest(http.MethodPost, "https://example.test/", nil)
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if beforeMark {
		t.Fatal("IsIdempotent true before MarkIdempotent")
	}
	if !afterMark {
		t.Fatal("IsIdempotent false after MarkIdempotent")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pipeline/ -run TestIdempotentMarker -v`
Expected: FAIL — `undefined: pipeline.MarkIdempotent`.

- [ ] **Step 3: Write minimal implementation**

```go
// pipeline/idempotent.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package pipeline

// idempotentKey is the unexported request-value key under which an idempotency
// marker is stored. Using a private zero-size type avoids collisions with other
// packages' request values.
type idempotentKey struct{}

// MarkIdempotent records that req is safe to retry even if its HTTP method is not
// inherently idempotent — for example a POST carrying an Idempotency-Key. The
// retry policy consults this via [IsIdempotent].
func MarkIdempotent(req *Request) { req.SetValue(idempotentKey{}, true) }

// IsIdempotent reports whether [MarkIdempotent] was called for req.
func IsIdempotent(req *Request) bool {
	v, ok := req.Value(idempotentKey{}).(bool)
	return ok && v
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pipeline/ -run TestIdempotentMarker -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pipeline/idempotent.go pipeline/idempotent_test.go
git commit -m "feat(pipeline): add idempotency markers for cross-policy coordination"
```

---

## Task 5: `idempotency` package — UUIDv4 key generation

**Files:**
- Create: `idempotency/doc.go`, `idempotency/policy.go`
- Test: `idempotency/policy_test.go`

Build the key generator first (it is the only non-trivial logic), then the policy
in Task 6.

- [ ] **Step 1: Write the failing test**

```go
// idempotency/policy_test.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package idempotency

import (
	"regexp"
	"testing"
)

var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewUUIDv4Format(t *testing.T) {
	t.Parallel()

	for i := 0; i < 100; i++ {
		got, err := newUUIDv4()
		if err != nil {
			t.Fatalf("newUUIDv4: %v", err)
		}
		if !uuidV4.MatchString(got) {
			t.Fatalf("newUUIDv4 = %q, not a canonical UUIDv4", got)
		}
	}
}

func TestNewUUIDv4Unique(t *testing.T) {
	t.Parallel()

	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		got, err := newUUIDv4()
		if err != nil {
			t.Fatalf("newUUIDv4: %v", err)
		}
		if _, dup := seen[got]; dup {
			t.Fatalf("duplicate UUID %q", got)
		}
		seen[got] = struct{}{}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./idempotency/ -run TestNewUUIDv4 -v`
Expected: FAIL — package/`newUUIDv4` undefined (compile error).

- [ ] **Step 3: Write minimal implementation**

```go
// idempotency/doc.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package idempotency provides a pipeline policy that stamps an Idempotency-Key
// header on requests whose method is not inherently idempotent (POST by
// default), so that the retry policy can safely re-send them. Keys are random
// UUIDv4 values generated from crypto/rand; a caller-supplied key is never
// overwritten.
package idempotency
```

```go
// idempotency/policy.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package idempotency

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// newUUIDv4 returns a random RFC 4122 version-4 UUID in canonical string form.
// It reads 16 bytes from crypto/rand and returns a wrapped error on failure,
// never a weak or empty key.
func newUUIDv4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("idempotency: read random bytes: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	var buf [36]byte
	hex.Encode(buf[0:8], b[0:4])
	buf[8] = '-'
	hex.Encode(buf[9:13], b[4:6])
	buf[13] = '-'
	hex.Encode(buf[14:18], b[6:8])
	buf[18] = '-'
	hex.Encode(buf[19:23], b[8:10])
	buf[23] = '-'
	hex.Encode(buf[24:36], b[10:16])
	return string(buf[:]), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./idempotency/ -run TestNewUUIDv4 -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add idempotency/doc.go idempotency/policy.go idempotency/policy_test.go
git commit -m "feat(idempotency): add crypto/rand UUIDv4 key generation"
```

---

## Task 6: `idempotency` policy

**Files:**
- Modify: `idempotency/policy.go`
- Test: `idempotency/policy_test.go`

The policy: for a matching method with no existing header, generate a key, set
the header, and call `pipeline.MarkIdempotent`. A caller-supplied header is left
untouched but the request is still marked idempotent (so retry treats it as
safe). On key-generation failure the policy returns the error without sending.

- [ ] **Step 1: Write the failing test**

```go
// append to idempotency/policy_test.go (add imports: "errors", "net/http",
// "strings", "github.com/dexpace/go-sdk/pipeline")

type transporterFunc func(*http.Request) (*http.Response, error)

func (f transporterFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

func okResp(req *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Body: http.NoBody, Request: req}, nil
}

func runPolicy(t *testing.T, p *Policy, req *http.Request) *http.Request {
	t.Helper()
	var captured *http.Request
	tr := transporterFunc(func(r *http.Request) (*http.Response, error) {
		captured = r
		return okResp(r)
	})
	pl := pipeline.New(tr, p)
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return captured
}

func TestPolicyStampsKeyOnPost(t *testing.T) {
	t.Parallel()

	p := NewPolicy(Options{})
	req, _ := http.NewRequest(http.MethodPost, "https://example.test/", strings.NewReader("x"))
	got := runPolicy(t, p, req)

	if key := got.Header.Get("Idempotency-Key"); !uuidV4.MatchString(key) {
		t.Fatalf("Idempotency-Key = %q, want a UUIDv4", key)
	}
}

func TestPolicySkipsGet(t *testing.T) {
	t.Parallel()

	p := NewPolicy(Options{})
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	got := runPolicy(t, p, req)

	if key := got.Header.Get("Idempotency-Key"); key != "" {
		t.Fatalf("Idempotency-Key = %q on GET, want empty", key)
	}
}

func TestPolicyKeepsCallerKey(t *testing.T) {
	t.Parallel()

	p := NewPolicy(Options{})
	req, _ := http.NewRequest(http.MethodPost, "https://example.test/", strings.NewReader("x"))
	req.Header.Set("Idempotency-Key", "caller-supplied")
	got := runPolicy(t, p, req)

	if key := got.Header.Get("Idempotency-Key"); key != "caller-supplied" {
		t.Fatalf("Idempotency-Key = %q, want caller-supplied", key)
	}
}

func TestPolicyMarksRequestIdempotent(t *testing.T) {
	t.Parallel()

	p := NewPolicy(Options{})
	var marked bool
	probe := pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		marked = pipeline.IsIdempotent(req)
		return req.Next()
	})
	tr := transporterFunc(okResp)
	pl := pipeline.New(tr, p, probe)
	req, _ := http.NewRequest(http.MethodPost, "https://example.test/", strings.NewReader("x"))
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if !marked {
		t.Fatal("request not marked idempotent")
	}
}

func TestPolicyKeyGenerationFailure(t *testing.T) {
	t.Parallel()

	p := NewPolicy(Options{NewKey: func() (string, error) {
		return "", errors.New("rng down")
	}})
	tr := transporterFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("transport reached despite key-generation failure")
		return nil, nil
	})
	pl := pipeline.New(tr, p)
	req, _ := http.NewRequest(http.MethodPost, "https://example.test/", strings.NewReader("x"))
	if _, err := pl.Do(req); err == nil {
		t.Fatal("expected error from key-generation failure")
	}
}
```

Note the `NewKey` field has signature `func() (string, error)`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./idempotency/ -v`
Expected: FAIL — `undefined: NewPolicy`, `Options`, `Policy`.

- [ ] **Step 3: Write minimal implementation**

```go
// append to idempotency/policy.go (add imports: "net/http",
// "github.com/dexpace/go-sdk/header", "github.com/dexpace/go-sdk/pipeline")

const defaultHeader = "Idempotency-Key"

// Options configures the idempotency [Policy]. The zero value is valid and
// yields the documented defaults: POST only, the "Idempotency-Key" header, and
// crypto/rand UUIDv4 keys.
type Options struct {
	// Methods lists the HTTP methods that receive a key. Nil selects ["POST"].
	// Method names are matched case-insensitively.
	Methods []string

	// Header is the header name to set. Empty selects "Idempotency-Key".
	Header string

	// NewKey generates a key. Nil selects a crypto/rand UUIDv4 generator.
	NewKey func() (string, error)
}

// Policy stamps an idempotency-key header on matching requests. It implements
// pipeline.Policy and is safe for concurrent use.
type Policy struct {
	methods map[string]struct{}
	header  string
	newKey  func() (string, error)
}

// NewPolicy returns an idempotency policy configured by opts.
func NewPolicy(opts Options) *Policy {
	methods := opts.Methods
	if methods == nil {
		methods = []string{http.MethodPost}
	}
	set := make(map[string]struct{}, len(methods))
	for _, m := range methods {
		set[strings.ToUpper(m)] = struct{}{}
	}
	h := opts.Header
	if h == "" {
		h = defaultHeader
	}
	newKey := opts.NewKey
	if newKey == nil {
		newKey = newUUIDv4
	}
	return &Policy{methods: set, header: h, newKey: newKey}
}

// Do implements pipeline.Policy.
func (p *Policy) Do(req *pipeline.Request) (*http.Response, error) {
	raw := req.Raw()
	if _, ok := p.methods[strings.ToUpper(raw.Method)]; !ok {
		return req.Next()
	}
	if raw.Header.Get(p.header) == "" {
		key, err := p.newKey()
		if err != nil {
			return nil, err
		}
		raw.Header.Set(p.header, key)
	}
	pipeline.MarkIdempotent(req)
	return req.Next()
}
```

Add `"strings"` to the `policy.go` import group as well.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./idempotency/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add idempotency/policy.go idempotency/policy_test.go
git commit -m "feat(idempotency): add idempotency-key policy with retry coordination"
```

---

## Task 7: Method-aware transport-error retry

**Files:**
- Modify: `retry/retry.go`
- Test: `retry/retry_test.go` (create if absent; otherwise append)

Today `retryableErr` retries every non-context transport error regardless of
method. Change `shouldRetry` to consult the request: on a transport error, retry
only when the method is safe/idempotent or the request is marked idempotent (or
carries a canonical `Idempotency-Key` header).

- [ ] **Step 1: Write the failing test**

```go
// retry/retry_test.go (create if it does not exist)
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package retry_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/dexpace/go-sdk/idempotency"
	"github.com/dexpace/go-sdk/pipeline"
	"github.com/dexpace/go-sdk/retry"
)

type transporterFunc func(*http.Request) (*http.Response, error)

func (f transporterFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

// countingTransport always fails with a transport error and records attempts.
func countingTransport(calls *int) transporterFunc {
	return func(*http.Request) (*http.Response, error) {
		*calls++
		return nil, errors.New("dial tcp: connection refused")
	}
}

func TestRetriesGetOnTransportError(t *testing.T) {
	t.Parallel()

	var calls int
	pl := pipeline.New(countingTransport(&calls),
		retry.NewPolicy(retry.Options{MaxRetries: 2, BaseDelay: 1, MaxDelay: 1}))
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	_, _ = pl.Do(req)

	if calls != 3 { // initial + 2 retries
		t.Fatalf("GET attempts = %d, want 3", calls)
	}
}

func TestDoesNotRetryUnkeyedPost(t *testing.T) {
	t.Parallel()

	var calls int
	pl := pipeline.New(countingTransport(&calls),
		retry.NewPolicy(retry.Options{MaxRetries: 2, BaseDelay: 1, MaxDelay: 1}))
	req, _ := http.NewRequest(http.MethodPost, "https://example.test/", nil)
	_, _ = pl.Do(req)

	if calls != 1 { // no retries for a non-idempotent POST
		t.Fatalf("unkeyed POST attempts = %d, want 1", calls)
	}
}

func TestRetriesKeyedPost(t *testing.T) {
	t.Parallel()

	var calls int
	pl := pipeline.New(countingTransport(&calls),
		retry.NewPolicy(retry.Options{MaxRetries: 2, BaseDelay: 1, MaxDelay: 1}),
		idempotency.NewPolicy(idempotency.Options{}))
	req, _ := http.NewRequest(http.MethodPost, "https://example.test/", nil)
	_, _ = pl.Do(req)

	if calls != 3 { // POST is now retry-safe because it carries an idempotency key
		t.Fatalf("keyed POST attempts = %d, want 3", calls)
	}
}
```

If `retry/retry_test.go` already exists with a `transporterFunc`, append only the
three test functions and the `countingTransport` helper, and add the
`idempotency` import.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./retry/ -run 'TestRetriesGetOnTransportError|TestDoesNotRetryUnkeyedPost|TestRetriesKeyedPost' -v`
Expected: FAIL — `TestDoesNotRetryUnkeyedPost` fails (current code retries the POST 3 times).

- [ ] **Step 3: Write minimal implementation**

Replace the `shouldRetry` method and the transport-error branch. In
`retry/retry.go`, change the signature so the request is available:

```go
// replace the existing shouldRetry with this version
func (p *Policy) shouldRetry(attempt int, req *pipeline.Request, resp *http.Response, err error) bool {
	if attempt >= p.opts.MaxRetries {
		return false
	}
	if err != nil {
		return retryableErr(err) && retrySafe(req)
	}
	return p.retryableStatus(resp.StatusCode)
}

// retrySafe reports whether a request may be re-sent after a transport error.
// Safe and idempotent methods always qualify; other methods (POST, PATCH) only
// qualify when an idempotency key makes the repeat safe.
func retrySafe(req *pipeline.Request) bool {
	switch req.Raw().Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions,
		http.MethodTrace, http.MethodPut, http.MethodDelete:
		return true
	}
	if pipeline.IsIdempotent(req) {
		return true
	}
	return req.Raw().Header.Get(idempotencyKeyHeader) != ""
}

const idempotencyKeyHeader = "Idempotency-Key"
```

Update the call site in `Do`:

```go
	resp, err = req.Next()
	if !p.shouldRetry(attempt, req, resp, err) {
		return resp, err
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./retry/ -v`
Expected: PASS (new tests pass; existing retry tests still pass).

- [ ] **Step 5: Update the Policy doc comment for accuracy**

In `retry/retry.go`, replace the `Policy` doc comment paragraph about transport
errors with:

```go
// On a transport error (the request never produced a response) the policy
// retries only requests that are safe to repeat: methods that are idempotent by
// definition (GET, HEAD, OPTIONS, TRACE, PUT, DELETE), or any request carrying an
// idempotency key (see package idempotency). On a response, the policy retries
// the configured status codes.
```

- [ ] **Step 6: Commit**

```bash
git add retry/retry.go retry/retry_test.go
git commit -m "fix(retry): make transport-error retries method-aware"
```

---

## Task 8: Transport redirect configuration

**Files:**
- Modify: `transport/transport.go`
- Test: `transport/transport_test.go` (create if absent; otherwise append)

Add `WithMaxRedirects` and `WithRedirectPolicy`, both setting
`http.Client.CheckRedirect` on the default client. They are ignored when
`WithClient` supplies a fully built client (matching `WithTimeout`/
`WithRoundTripper`). `WithMaxRedirects(0)` stops following redirects and returns
the redirect response.

- [ ] **Step 1: Write the failing test**

```go
// transport/transport_test.go (create if it does not exist)
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package transport_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dexpace/go-sdk/transport"
)

func TestWithMaxRedirectsZeroDoesNotFollow(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/dest", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	tr := transport.New(transport.WithMaxRedirects(0))
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/start", nil)
	resp, err := tr.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302 (redirect not followed)", resp.StatusCode)
	}
}

func TestWithMaxRedirectsCaps(t *testing.T) {
	t.Parallel()

	// Each hop redirects to the next; with a cap of 2 the third hop must error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/next", http.StatusFound)
	}))
	t.Cleanup(srv.Close)

	tr := transport.New(transport.WithMaxRedirects(2))
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/start", nil)
	resp, err := tr.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("expected error after exceeding redirect cap")
	}
}

func TestWithRedirectPolicyCustom(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/dest", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusTeapot)
	}))
	t.Cleanup(srv.Close)

	var hops int
	tr := transport.New(transport.WithRedirectPolicy(func(req *http.Request, via []*http.Request) error {
		hops = len(via)
		return nil // follow
	}))
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/start", nil)
	resp, err := tr.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusTeapot {
		t.Fatalf("status = %d, want 418 (redirect followed)", resp.StatusCode)
	}
	if hops != 1 {
		t.Fatalf("custom policy saw %d prior requests, want 1", hops)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./transport/ -run 'WithMaxRedirects|WithRedirectPolicy' -v`
Expected: FAIL — `undefined: transport.WithMaxRedirects`.

- [ ] **Step 3: Write minimal implementation**

```go
// in transport/transport.go, add a field to config:
type config struct {
	client        *http.Client
	roundTripper  http.RoundTripper
	timeout       time.Duration
	checkRedirect func(req *http.Request, via []*http.Request) error
}

// WithMaxRedirects caps how many redirects the default client follows. A value
// of 0 stops following redirects and returns the redirect response itself.
// Ignored when [WithClient] is supplied. net/http already strips sensitive
// headers (Authorization, Cookie) on cross-origin redirects.
func WithMaxRedirects(n int) Option {
	return func(cfg *config) {
		cfg.checkRedirect = func(_ *http.Request, via []*http.Request) error {
			if n <= 0 {
				return http.ErrUseLastResponse
			}
			if len(via) >= n {
				return fmt.Errorf("transport: stopped after %d redirects", n)
			}
			return nil
		}
	}
}

// WithRedirectPolicy sets a custom redirect policy on the default client,
// mirroring http.Client.CheckRedirect: return nil to follow, http.ErrUseLastResponse
// to stop and return the last response, or any other error to fail the call.
// Ignored when [WithClient] is supplied.
func WithRedirectPolicy(fn func(req *http.Request, via []*http.Request) error) Option {
	return func(cfg *config) { cfg.checkRedirect = fn }
}
```

Wire it into `New` where the default client is built:

```go
	return &Transport{client: &http.Client{
		Transport:     rt,
		Timeout:       cfg.timeout,
		CheckRedirect: cfg.checkRedirect,
	}}
```

Add `"fmt"` to the import group.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./transport/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add transport/transport.go transport/transport_test.go
git commit -m "feat(transport): add WithMaxRedirects and WithRedirectPolicy"
```

---

## Task 9: Umbrella set-date policy and `WithDate`

**Files:**
- Modify: `client.go`, `options.go`
- Test: `client_test.go` (create if absent; otherwise append)

Add a small inline `datePolicy` and a `WithDate()` option. The policy stamps the
`Date` header in RFC 1123 form (`http.TimeFormat`) when absent.

- [ ] **Step 1: Write the failing test**

```go
// client_test.go (create if it does not exist)
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package dexpace_test

import (
	"net/http"
	"testing"

	dexpace "github.com/dexpace/go-sdk"
)

type transporterFunc func(*http.Request) (*http.Response, error)

func (f transporterFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

func captureTransport(captured **http.Request) transporterFunc {
	return func(r *http.Request) (*http.Response, error) {
		*captured = r
		return &http.Response{StatusCode: 200, Body: http.NoBody, Request: r}, nil
	}
}

func TestWithDateStampsHeader(t *testing.T) {
	t.Parallel()

	var captured *http.Request
	c := dexpace.New(
		dexpace.WithTransport(captureTransport(&captured)),
		dexpace.WithDate(),
	)
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if captured.Header.Get("Date") == "" {
		t.Fatal("Date header not set with WithDate()")
	}
}

func TestDateOffByDefault(t *testing.T) {
	t.Parallel()

	var captured *http.Request
	c := dexpace.New(dexpace.WithTransport(captureTransport(&captured)))
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if captured.Header.Get("Date") != "" {
		t.Fatal("Date header set without WithDate()")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run 'TestWithDateStampsHeader|TestDateOffByDefault' -v`
Expected: FAIL — `undefined: dexpace.WithDate`.

- [ ] **Step 3: Write minimal implementation**

In `options.go`, add a `date bool` field to `config` and the option:

```go
// WithDate stamps a Date header (RFC 1123) on each request that lacks one. Off
// by default; net/http does not set a request Date and most REST APIs do not
// need it, but some request-signing schemes require it.
func WithDate() Option {
	return func(c *config) { c.date = true }
}
```

In `client.go`, add the inline policy near `userAgentPolicy`:

```go
// datePolicy stamps the Date header (RFC 1123) unless the caller already set one.
func datePolicy() pipeline.Policy {
	return pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		if req.Raw().Header.Get(header.Date) == "" {
			req.Raw().Header.Set(header.Date, time.Now().UTC().Format(http.TimeFormat))
		}
		return req.Next()
	})
}
```

This needs a `header.Date` constant. If it does not exist, add to
`header/header.go`:

```go
// Date is the canonical Date header name.
Date = "Date"
```

Add `"time"` to the `client.go` import group. (Final wiring of `datePolicy` into
the stack happens in Task 10; for now the option records intent and the policy
exists.)

- [ ] **Step 4: Wire it into the existing fixed stack temporarily**

So the test passes before the Task 10 rewrite, append the date policy in
`New` after the user-agent policy:

```go
	policies = append(policies, userAgentPolicy(ua))
	if cfg.date {
		policies = append(policies, datePolicy())
	}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test . -run 'TestWithDateStampsHeader|TestDateOffByDefault' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add client.go options.go header/header.go client_test.go
git commit -m "feat: add opt-in Date header policy via WithDate"
```

---

## Task 10: Stage-based assembly in the umbrella

**Files:**
- Modify: `client.go`, `options.go`
- Test: `client_test.go`

Rewire `New` to assemble built-ins as `pipeline.Placement`s and call
`pipeline.NewStaged`. Add `WithoutIdempotency`, `WithIdempotency`,
`WithPolicyBefore`, `WithPolicyAfter`. Idempotency is **default-on**.

- [ ] **Step 1: Write the failing test**

```go
// append to client_test.go (add imports: "strings",
// "github.com/dexpace/go-sdk/pipeline")

func TestIdempotencyOnByDefaultForPost(t *testing.T) {
	t.Parallel()

	var captured *http.Request
	c := dexpace.New(dexpace.WithTransport(captureTransport(&captured)))
	req, _ := http.NewRequest(http.MethodPost, "https://example.test/", strings.NewReader("x"))
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if captured.Header.Get("Idempotency-Key") == "" {
		t.Fatal("Idempotency-Key not set by default on POST")
	}
}

func TestWithoutIdempotency(t *testing.T) {
	t.Parallel()

	var captured *http.Request
	c := dexpace.New(
		dexpace.WithTransport(captureTransport(&captured)),
		dexpace.WithoutIdempotency(),
	)
	req, _ := http.NewRequest(http.MethodPost, "https://example.test/", strings.NewReader("x"))
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if captured.Header.Get("Idempotency-Key") != "" {
		t.Fatal("Idempotency-Key set despite WithoutIdempotency()")
	}
}

func TestWithPolicyBeforeAndAfterRun(t *testing.T) {
	t.Parallel()

	var ran []string
	mk := func(name string) pipeline.Policy {
		return pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
			ran = append(ran, name)
			return req.Next()
		})
	}
	var captured *http.Request
	c := dexpace.New(
		dexpace.WithTransport(captureTransport(&captured)),
		dexpace.WithPolicyBefore(pipeline.StageRetry, mk("before-retry")),
		dexpace.WithPolicyAfter(pipeline.StageAuth, mk("after-auth")),
	)
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if len(ran) != 2 || ran[0] != "before-retry" || ran[1] != "after-auth" {
		t.Fatalf("custom policies ran = %v, want [before-retry after-auth]", ran)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run 'TestIdempotencyOnByDefaultForPost|TestWithoutIdempotency|TestWithPolicyBeforeAndAfterRun' -v`
Expected: FAIL — `undefined: dexpace.WithoutIdempotency`.

- [ ] **Step 3: Extend `options.go`**

Add fields and options:

```go
// add to the config struct:
	noIdempotency bool
	idempotency   *idempotency.Options
	before        []pipeline.Placement
	after         []pipeline.Placement
```

```go
// WithoutIdempotency disables the default idempotency-key policy.
func WithoutIdempotency() Option {
	return func(c *config) { c.noIdempotency = true }
}

// WithIdempotency configures the idempotency-key policy (which is on by default).
// Passing custom options also re-enables it if a prior WithoutIdempotency was set.
func WithIdempotency(opts idempotency.Options) Option {
	return func(c *config) {
		c.idempotency = &opts
		c.noIdempotency = false
	}
}

// WithPolicyBefore inserts a custom policy immediately before the given stage.
func WithPolicyBefore(stage pipeline.Stage, p pipeline.Policy) Option {
	return func(c *config) { c.before = append(c.before, pipeline.Before(stage, p)) }
}

// WithPolicyAfter inserts a custom policy immediately after the given stage.
func WithPolicyAfter(stage pipeline.Stage, p pipeline.Policy) Option {
	return func(c *config) { c.after = append(c.after, pipeline.After(stage, p)) }
}
```

Add imports `"github.com/dexpace/go-sdk/idempotency"` and
`"github.com/dexpace/go-sdk/pipeline"` to `options.go`.

- [ ] **Step 4: Rewrite `New` in `client.go` to use placements**

Replace the policy-assembly block (the `policies := make(...)` section and the
`pipeline.New(t, policies...)` call, including the temporary Task 9 wiring) with:

```go
	placements := []pipeline.Placement{
		pipeline.At(pipeline.StageClientIdentity, userAgentPolicy(ua)),
		pipeline.At(pipeline.StageRetry, retry.NewPolicy(retryOpts)),
	}

	if !cfg.noIdempotency {
		iopts := idempotency.Options{}
		if cfg.idempotency != nil {
			iopts = *cfg.idempotency
		}
		placements = append(placements,
			pipeline.At(pipeline.StageIdempotency, idempotency.NewPolicy(iopts)))
	}
	if cfg.credential != nil {
		placements = append(placements,
			pipeline.At(pipeline.StageAuth, auth.NewBearerTokenPolicy(cfg.credential, cfg.scopes...)))
	}
	if cfg.date {
		placements = append(placements, pipeline.At(pipeline.StageDate, datePolicy()))
	}
	if cfg.logging {
		placements = append(placements,
			pipeline.At(pipeline.StageLogging, logging.NewPolicy(logging.Options{Logger: cfg.logger})))
	}
	placements = append(placements, cfg.before...)
	placements = append(placements, cfg.after...)
	for _, p := range cfg.custom {
		// Custom WithPolicies land innermost, just before transport — preserve
		// today's behavior by anchoring them after the innermost stage.
		placements = append(placements, pipeline.After(pipeline.StageLogging, p))
	}

	return &Client{pl: pipeline.NewStaged(t, placements...)}
```

Add `"github.com/dexpace/go-sdk/idempotency"` to the `client.go` imports. Update
the `New` doc comment to describe the stage order:

```go
// New assembles a Client. Built-in policies are placed in stage order, outermost
// first:
//
//	client-identity → idempotency → retry → auth → date → logging → transport
//
// Retry wraps the inner stages, so auth re-runs (and may refresh its token) on
// every attempt; logging is innermost, so it records the request as sent.
// Idempotency-key stamping is on by default for POST (disable with
// WithoutIdempotency); set-date is opt-in (WithDate).
```

- [ ] **Step 5: Run the full test suite**

Run: `go test ./... -v`
Expected: PASS — including the Task 9 date tests (now driven through stages) and
all pre-existing tests.

- [ ] **Step 6: Commit**

```bash
git add client.go options.go client_test.go
git commit -m "feat: assemble client pipeline via stage placements"
```

---

## Task 11: Document the stage model and run the full gate

**Files:**
- Modify: `pipeline/doc.go`

- [ ] **Step 1: Read the current package doc**

Run: `cat pipeline/doc.go`
Expected: see the existing package comment to extend (do not duplicate the
license header; edit in place).

- [ ] **Step 2: Add a stage-model section to the package doc**

Append to the package doc comment in `pipeline/doc.go` (keep it above the
`package pipeline` clause, no blank line between comment and clause):

```go
// # Stage ordering
//
// Most callers build a pipeline through the umbrella package's options, but the
// ordering is expressed here. A [Stage] names an anchor point in the standard
// policy order; [At], [Before], and [After] position a [Policy] relative to a
// stage; [NewStaged] resolves those placements into the flat ordered list that
// [New] runs. Resolution is stable: placements are ordered by stage, then by
// before/at/after, and ties keep insertion order. Placing two policies At the
// same stage runs them in insertion order (the second effectively "after" the
// first), which is how a built-in pillar is replaced or supplemented.
```

- [ ] **Step 3: Run formatting, vet, lint, and the race suite**

Run:
```bash
gofumpt -l -w . && goimports -w . && go vet ./... && golangci-lint run && go test -race ./...
```
Expected: no formatting diffs reported as errors, vet clean, lint clean, all
tests pass under the race detector.

- [ ] **Step 4: Commit**

```bash
git add pipeline/doc.go
git commit -m "docs(pipeline): document the stage ordering model"
```

---

## Self-Review notes (for the implementer)

- **Spec coverage:** Stage model (Tasks 1–3, 11), idempotency coordination
  (Task 4), idempotency policy (Tasks 5–6), method-aware retry (Task 7),
  transport redirect config (Task 8), set-date (Task 9), stage assembly + new
  options (Task 10). All spec sections map to a task.
- **Behavior changes to surface in release notes:** logging now innermost;
  idempotency-key default-on for POST; transport-error retries now method-aware
  (unkeyed POST/PATCH no longer retried, keyed POST now retried).
- **Type consistency:** `Options.NewKey` is `func() (string, error)` everywhere;
  `pipeline.MarkIdempotent`/`IsIdempotent` used in Tasks 4, 6, 7; `Stage`
  constants used in Tasks 1, 3, 10; `Placement` from `At`/`Before`/`After` used
  in Tasks 2, 3, 10.
- **`make check`** (`tidy + fmt + vet + lint + test`) should be green before
  opening a PR.
