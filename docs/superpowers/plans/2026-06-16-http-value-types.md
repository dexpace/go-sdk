# HTTP Value Types Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add conditional-request value types — `ETag`, `Range`, and `Conditions` — in a new `conditions` package, plus the missing `header` constants.

**Architecture:** Immutable value types with constructors and an `Apply(*http.Request)` that stamps headers (consistent with `mediatype`). No umbrella or policy wiring — these are per-request helpers.

**Tech Stack:** Go 1.26+, standard library only (`fmt`, `net/http`, `strings`, `time`). Zero third-party dependencies.

**Conventions every task must follow:**
- MIT license header on every `.go` file before the `package` clause:
  ```go
  // Copyright (c) 2026 dexpace and Omar Aljarrah.
  // Licensed under the MIT License. See LICENSE in the repository root for details.
  ```
- Import groups: stdlib, blank line, then `github.com/dexpace/go-sdk/...`.
- Tests use `t.Parallel()`; table-driven; stdlib-only.
- Tools: Go 1.26.3; `gofumpt`/`golangci-lint` NOT installed — use `gofmt`, `go vet`, `go test -race`.
- Run commands from the repo root `/Users/omar/dexpace/go-sdk`.

---

## File Structure

| Path | Responsibility |
|---|---|
| `header/header.go` (modify) | add `Range`, `IfModifiedSince`, `IfUnmodifiedSince` |
| `header/header_test.go` (new) | assert the new constants' canonical strings |
| `conditions/doc.go` (new) | package comment |
| `conditions/etag.go` (new) + test | `ETag` |
| `conditions/range.go` (new) + test | `Range` |
| `conditions/conditions.go` (new) + test | `Conditions` + `Apply` |
| `doc.go`, `README.md`, `CLAUDE.md` (modify) | document; add the package |

---

## Task 1: header constants, `ETag`, and `Range`

**Files:**
- Modify: `header/header.go`
- Create: `header/header_test.go`, `conditions/doc.go`, `conditions/etag.go`, `conditions/etag_test.go`, `conditions/range.go`, `conditions/range_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// header/header_test.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package header_test

import (
	"net/http"
	"testing"

	"github.com/dexpace/go-sdk/header"
)

func TestNewHeaderConstantsAreCanonical(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		header.Range:             "Range",
		header.IfModifiedSince:   "If-Modified-Since",
		header.IfUnmodifiedSince: "If-Unmodified-Since",
	}
	for got, want := range cases {
		if got != want {
			t.Fatalf("constant = %q, want %q", got, want)
		}
		if canon := http.CanonicalHeaderKey(want); canon != got {
			t.Fatalf("constant %q is not canonical (canonical is %q)", got, canon)
		}
	}
}
```

```go
// conditions/etag_test.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package conditions_test

import (
	"testing"

	"github.com/dexpace/go-sdk/conditions"
)

func TestETagString(t *testing.T) {
	t.Parallel()

	if got := conditions.NewETag("abc").String(); got != `"abc"` {
		t.Fatalf("strong ETag = %q, want \"abc\"", got)
	}
	if got := conditions.NewWeakETag("abc").String(); got != `W/"abc"` {
		t.Fatalf("weak ETag = %q, want W/\"abc\"", got)
	}
}

func TestETagParse(t *testing.T) {
	t.Parallel()

	strong, err := conditions.Parse(`"abc"`)
	if err != nil {
		t.Fatalf("Parse strong: %v", err)
	}
	if strong.Tag() != "abc" || strong.Weak() {
		t.Fatalf("strong = %+v, want tag=abc weak=false", strong)
	}

	weak, err := conditions.Parse(`W/"abc"`)
	if err != nil {
		t.Fatalf("Parse weak: %v", err)
	}
	if weak.Tag() != "abc" || !weak.Weak() {
		t.Fatalf("weak = %+v, want tag=abc weak=true", weak)
	}

	for _, bad := range []string{"", "abc", `"abc`, `abc"`, "W/abc"} {
		if _, err := conditions.Parse(bad); err == nil {
			t.Fatalf("Parse(%q) should fail", bad)
		}
	}
}

func TestETagRoundTrip(t *testing.T) {
	t.Parallel()

	for _, e := range []conditions.ETag{conditions.NewETag("x"), conditions.NewWeakETag("y")} {
		got, err := conditions.Parse(e.String())
		if err != nil {
			t.Fatalf("Parse(%q): %v", e.String(), err)
		}
		if got != e {
			t.Fatalf("round-trip = %+v, want %+v", got, e)
		}
	}
}
```

```go
// conditions/range_test.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package conditions_test

import (
	"net/http"
	"testing"

	"github.com/dexpace/go-sdk/conditions"
)

func TestRangeString(t *testing.T) {
	t.Parallel()

	if got := conditions.Bytes(0, 99).String(); got != "bytes=0-99" {
		t.Fatalf("Bytes(0,99) = %q, want bytes=0-99", got)
	}
	if got := conditions.BytesFrom(100).String(); got != "bytes=100-" {
		t.Fatalf("BytesFrom(100) = %q, want bytes=100-", got)
	}
}

func TestRangeApply(t *testing.T) {
	t.Parallel()

	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
	conditions.Bytes(0, 1023).Apply(req)
	if got := req.Header.Get("Range"); got != "bytes=0-1023" {
		t.Fatalf("Range header = %q, want bytes=0-1023", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./header/ ./conditions/ -v`
Expected: FAIL — new header constants undefined; `conditions` package has no `ETag`/`Range`.

- [ ] **Step 3: Add header constants**

In `header/header.go`, add to the `const (...)` block (keep gofmt alignment; place alphabetically near the other `If*` and `R*` entries):

```go
	IfModifiedSince   = "If-Modified-Since"
	IfUnmodifiedSince = "If-Unmodified-Since"
	Range             = "Range"
```

- [ ] **Step 4: Create the `conditions` package doc and `ETag`**

```go
// conditions/doc.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package conditions provides immutable value types for conditional and range
// requests — entity tags ([ETag]), byte ranges ([Range]), and the precondition
// header set ([Conditions]) — each of which stamps the appropriate headers on an
// *http.Request via its Apply method (or, for ETag, its String form).
package conditions
```

```go
// conditions/etag.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package conditions

import (
	"fmt"
	"strings"
)

// ETag is an HTTP entity-tag validator (RFC 9110). The tag is the opaque value
// without surrounding quotes; a weak tag is rendered with a leading "W/".
type ETag struct {
	tag  string
	weak bool
}

// NewETag returns a strong entity tag.
func NewETag(tag string) ETag { return ETag{tag: tag} }

// NewWeakETag returns a weak entity tag.
func NewWeakETag(tag string) ETag { return ETag{tag: tag, weak: true} }

// Parse parses an entity tag in wire form, "abc" or W/"abc", returning an error
// for input that is not a (optionally W/-prefixed) quoted string.
func Parse(s string) (ETag, error) {
	weak := false
	if rest, ok := strings.CutPrefix(s, "W/"); ok {
		weak = true
		s = rest
	}
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return ETag{}, fmt.Errorf("conditions: invalid ETag %q", s)
	}
	return ETag{tag: s[1 : len(s)-1], weak: weak}, nil
}

// Tag returns the opaque tag value without quotes.
func (e ETag) Tag() string { return e.tag }

// Weak reports whether the tag is a weak validator.
func (e ETag) Weak() bool { return e.weak }

// String returns the wire form: "abc" for a strong tag, W/"abc" for a weak one.
func (e ETag) String() string {
	quoted := `"` + e.tag + `"`
	if e.weak {
		return "W/" + quoted
	}
	return quoted
}
```

- [ ] **Step 5: Create `Range`**

```go
// conditions/range.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package conditions

import (
	"fmt"
	"net/http"

	"github.com/dexpace/go-sdk/header"
)

// Range is an HTTP byte range for the Range header (RFC 9110 §14.2).
type Range struct {
	start  int64
	end    int64
	hasEnd bool
}

// Bytes returns the inclusive byte range [start, end].
func Bytes(start, end int64) Range {
	return Range{start: start, end: end, hasEnd: true}
}

// BytesFrom returns the open-ended byte range [start, end-of-resource).
func BytesFrom(start int64) Range {
	return Range{start: start}
}

// String returns the Range header value, "bytes=start-end" or "bytes=start-".
func (r Range) String() string {
	if r.hasEnd {
		return fmt.Sprintf("bytes=%d-%d", r.start, r.end)
	}
	return fmt.Sprintf("bytes=%d-", r.start)
}

// Apply sets the Range header on req.
func (r Range) Apply(req *http.Request) {
	req.Header.Set(header.Range, r.String())
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./header/ ./conditions/ -v`
Expected: PASS — header constant test and all ETag/Range tests.

- [ ] **Step 7: Commit**

```bash
git add header/header.go header/header_test.go conditions/doc.go conditions/etag.go conditions/etag_test.go conditions/range.go conditions/range_test.go
git commit -m "feat(conditions): add ETag and Range value types; header constants"
```

---

## Task 2: `Conditions` and `Apply`

**Files:**
- Create: `conditions/conditions.go`, `conditions/conditions_test.go`

- [ ] **Step 1: Write the failing test**

```go
// conditions/conditions_test.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package conditions_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/dexpace/go-sdk/conditions"
)

func TestConditionsApplyIfNoneMatch(t *testing.T) {
	t.Parallel()

	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
	conditions.Conditions{
		IfNoneMatch: []conditions.ETag{conditions.NewETag("a"), conditions.NewWeakETag("b")},
	}.Apply(req)

	if got := req.Header.Get("If-None-Match"); got != `"a", W/"b"` {
		t.Fatalf("If-None-Match = %q, want \"a\", W/\"b\"", got)
	}
	if req.Header.Get("If-Match") != "" {
		t.Fatal("If-Match should be unset")
	}
}

func TestConditionsApplyModifiedSince(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
	conditions.Conditions{IfModifiedSince: ts}.Apply(req)

	if got := req.Header.Get("If-Modified-Since"); got != ts.Format(http.TimeFormat) {
		t.Fatalf("If-Modified-Since = %q, want %q", got, ts.Format(http.TimeFormat))
	}
}

func TestConditionsApplyEmptyIsNoOp(t *testing.T) {
	t.Parallel()

	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
	conditions.Conditions{}.Apply(req)

	for _, h := range []string{"If-Match", "If-None-Match", "If-Modified-Since", "If-Unmodified-Since"} {
		if req.Header.Get(h) != "" {
			t.Fatalf("%s should be unset for empty Conditions", h)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./conditions/ -run TestConditions -v`
Expected: FAIL — `conditions.Conditions` undefined.

- [ ] **Step 3: Write the implementation**

```go
// conditions/conditions.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package conditions

import (
	"net/http"
	"strings"
	"time"

	"github.com/dexpace/go-sdk/header"
)

// Conditions carries conditional-request headers (RFC 9110 §13). Empty ETag
// slices and zero times are left unset.
type Conditions struct {
	IfMatch           []ETag
	IfNoneMatch       []ETag
	IfModifiedSince   time.Time
	IfUnmodifiedSince time.Time
}

// Apply sets the configured conditional headers on req. Each ETag list is
// comma-joined; times are formatted as HTTP-dates in UTC. Unset fields leave the
// corresponding header untouched; set fields overwrite any existing value.
func (c Conditions) Apply(req *http.Request) {
	if v := joinETags(c.IfMatch); v != "" {
		req.Header.Set(header.IfMatch, v)
	}
	if v := joinETags(c.IfNoneMatch); v != "" {
		req.Header.Set(header.IfNoneMatch, v)
	}
	if !c.IfModifiedSince.IsZero() {
		req.Header.Set(header.IfModifiedSince, c.IfModifiedSince.UTC().Format(http.TimeFormat))
	}
	if !c.IfUnmodifiedSince.IsZero() {
		req.Header.Set(header.IfUnmodifiedSince, c.IfUnmodifiedSince.UTC().Format(http.TimeFormat))
	}
}

func joinETags(tags []ETag) string {
	if len(tags) == 0 {
		return ""
	}
	parts := make([]string, len(tags))
	for i, t := range tags {
		parts[i] = t.String()
	}
	return strings.Join(parts, ", ")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./conditions/ -v`
Expected: PASS — all Conditions tests plus ETag/Range.

- [ ] **Step 5: Run the full suite**

Run: `go test ./...`
Expected: PASS across every package.

- [ ] **Step 6: Commit**

```bash
git add conditions/conditions.go conditions/conditions_test.go
git commit -m "feat(conditions): add Conditions precondition headers"
```

---

## Task 3: docs and full gate

**Files:**
- Modify: `doc.go`, `README.md`, `CLAUDE.md`

- [ ] **Step 1: Mention conditions in `doc.go`**

Read `doc.go`. Within the `package dexpace` doc comment (single contiguous `//`
block; no second package clause / no duplicate header), add:

```go
// The conditions package provides value types for conditional and range requests
// (ETag, Range, Conditions) that stamp the appropriate headers on a request.
```

- [ ] **Step 2: Add `conditions` to `README.md`**

Read `README.md`. Add a `conditions` row to the architecture/package table
(matching the table's column/link style): "Conditional- and range-request value
types (ETag, Range, Conditions)."

- [ ] **Step 3: Add `conditions/` to `CLAUDE.md` Repository Layout**

Read `CLAUDE.md`. Add a `conditions/` line near the other value-layer packages
in the Repository Layout tree: `conditions/ # ETag, Range, Conditions value types`.

- [ ] **Step 4: Run the full gate**

Run:
```bash
gofmt -l .
go vet ./...
go test -race ./...
```
Expected: `gofmt -l .` prints nothing; `go vet` clean; every package passes under the race detector (`conditions` and `header` now have tests).

- [ ] **Step 5: Commit**

```bash
git add doc.go README.md CLAUDE.md
git commit -m "docs: document the conditions package"
```

---

## Self-Review notes (for the implementer)

- **Spec coverage:** header constants + ETag + Range (Task 1); Conditions + Apply (Task 2); docs (Task 3). Deferred items (multipart, JSONL/chunked) are intentionally not implemented.
- **Type consistency:** `conditions.NewETag/NewWeakETag/Parse`, `ETag.Tag/Weak/String`, `conditions.Bytes/BytesFrom`, `Range.String/Apply`, `conditions.Conditions{IfMatch,IfNoneMatch,IfModifiedSince,IfUnmodifiedSince}.Apply`, and `header.Range/IfModifiedSince/IfUnmodifiedSince` are used identically across tasks.
- **Value-type immutability:** all types are values with no exported mutable fields except `Conditions` (a plain config struct, mutation before Apply is the intended use).
- **`make check`** green before opening the PR.
