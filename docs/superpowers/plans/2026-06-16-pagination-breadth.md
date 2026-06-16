# Pagination Breadth Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `WithMaxPages` cap, page-number and Link-header pagination strategy constructors, and an RFC 8288 `NextLink` parser to the `pagination` package — all built on the existing token-based `Pager[T]`.

**Architecture:** The opaque `token` of `FetchFunc[T]` already generalizes cursor pagination. `NewPageNumber` and `NewLinkHeader` wrap a friendlier fetch signature into a `FetchFunc[T]` that computes `NextToken`. `WithMaxPages` bounds iteration in `Pages` (and therefore `Items`).

**Tech Stack:** Go 1.26+, standard library only (`context`, `iter`, `strconv`, `fmt`, `net/http`, `strings`). Zero third-party dependencies.

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
| `pagination/pagination.go` (modify) | `Option`, `WithMaxPages`, variadic `New`, page cap in `Pages` |
| `pagination/pagination_test.go` (modify) | cap test |
| `pagination/strategies.go` (new) | `NewPageNumber`, `NewLinkHeader`, `NextLink` |
| `pagination/strategies_test.go` (new) | page-number, Link-header, NextLink parser tests |

---

## Task 1: `WithMaxPages` cap

**Files:**
- Modify: `pagination/pagination.go`
- Test: `pagination/pagination_test.go`

The current `Pager[T]` is `type Pager[T any] struct { fetch FetchFunc[T] }`, built
by `func New[T any](fetch FetchFunc[T]) *Pager[T]`, and `Pages` loops until an empty
`NextToken`. Add a non-generic `Option`/`pagerConfig`, a `maxPages` field, and a cap.

- [ ] **Step 1: Write the failing test**

Append to `pagination/pagination_test.go`:

```go
func TestWithMaxPagesCapsIteration(t *testing.T) {
	t.Parallel()

	// A fetch that never ends (always a non-empty NextToken).
	fetch := func(_ context.Context, _ string) (pagination.Page[int], error) {
		return pagination.Page[int]{Items: []int{1}, NextToken: "more"}, nil
	}
	pager := pagination.New(fetch, pagination.WithMaxPages(3))

	pages := 0
	for _, err := range pager.Pages(context.Background()) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		pages++
	}
	if pages != 3 {
		t.Fatalf("pages = %d, want 3 (capped)", pages)
	}
}

func TestWithMaxPagesAlsoCapsItems(t *testing.T) {
	t.Parallel()

	fetch := func(_ context.Context, _ string) (pagination.Page[int], error) {
		return pagination.Page[int]{Items: []int{1, 2}, NextToken: "more"}, nil
	}
	pager := pagination.New(fetch, pagination.WithMaxPages(2))

	count := 0
	for _, err := range pager.Items(context.Background()) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		count++
	}
	if count != 4 { // 2 pages * 2 items
		t.Fatalf("items = %d, want 4 (2 pages capped)", count)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pagination/ -run TestWithMaxPages -v`
Expected: FAIL — `pagination.WithMaxPages` undefined.

- [ ] **Step 3: Modify `pagination/pagination.go`**

Add the option type and config (place them after the `FetchFunc` type):

```go
// Option configures a [Pager].
type Option func(*pagerConfig)

type pagerConfig struct {
	maxPages int
}

// WithMaxPages caps how many pages a Pager yields. A value <= 0 means unlimited
// (the default).
func WithMaxPages(n int) Option {
	return func(c *pagerConfig) { c.maxPages = n }
}
```

Change the `Pager` struct to carry the cap:

```go
// Pager lazily walks every page produced by a [FetchFunc].
type Pager[T any] struct {
	fetch    FetchFunc[T]
	maxPages int
}
```

Make `New` variadic and apply options:

```go
// New returns a Pager driven by fetch. Options such as [WithMaxPages] tune it.
func New[T any](fetch FetchFunc[T], opts ...Option) *Pager[T] {
	var c pagerConfig
	for _, opt := range opts {
		opt(&c)
	}
	return &Pager[T]{fetch: fetch, maxPages: c.maxPages}
}
```

Enforce the cap in `Pages` — add a page counter and a check at the top of the loop:

```go
// Pages returns an iterator over successive pages. Iteration stops after the
// page whose NextToken is empty, when ctx is cancelled, when fetch returns an
// error (delivered as the second value of the final iteration), or when the
// WithMaxPages cap is reached. The iterator is single-pass.
func (p *Pager[T]) Pages(ctx context.Context) iter.Seq2[Page[T], error] {
	return func(yield func(Page[T], error) bool) {
		token := ""
		pages := 0
		for {
			if p.maxPages > 0 && pages >= p.maxPages {
				return
			}
			if err := ctx.Err(); err != nil {
				yield(Page[T]{}, err)
				return
			}
			page, err := p.fetch(ctx, token)
			if err != nil {
				yield(Page[T]{}, err)
				return
			}
			if !yield(page, nil) {
				return
			}
			pages++
			if page.NextToken == "" {
				return
			}
			token = page.NextToken
		}
	}
}
```

(Keep `Items` unchanged — it iterates `Pages`, so the cap applies automatically.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pagination/ -v`
Expected: PASS — the two new cap tests plus all existing pagination tests (which call `New(fetch)` with no options).

- [ ] **Step 5: Commit**

```bash
git add pagination/pagination.go pagination/pagination_test.go
git commit -m "feat(pagination): add WithMaxPages cap"
```

---

## Task 2: page-number and Link-header strategies, `NextLink`

**Files:**
- Create: `pagination/strategies.go`, `pagination/strategies_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// pagination/strategies_test.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package pagination_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/dexpace/go-sdk/pagination"
)

func collect(t *testing.T, seq func(yield func(int, error) bool)) []int {
	t.Helper()
	var got []int
	for item, err := range seq {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got = append(got, item)
	}
	return got
}

func TestNewPageNumber(t *testing.T) {
	t.Parallel()

	// Pages 1..3 have items; page 4 is empty (stop).
	fetch := func(_ context.Context, page int) ([]int, error) {
		switch page {
		case 1:
			return []int{1, 2}, nil
		case 2:
			return []int{3, 4}, nil
		case 3:
			return []int{5}, nil
		default:
			return nil, nil // empty page → stop
		}
	}
	pager := pagination.NewPageNumber(1, fetch)
	got := collect(t, pager.Items(context.Background()))

	want := []int{1, 2, 3, 4, 5}
	if len(got) != len(want) {
		t.Fatalf("items = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("items = %v, want %v", got, want)
		}
	}
}

func TestNewLinkHeader(t *testing.T) {
	t.Parallel()

	respWithNext := func(next string) *http.Response {
		h := http.Header{}
		if next != "" {
			h.Set("Link", "<"+next+">; rel=\"next\"")
		}
		return &http.Response{Header: h}
	}

	fetch := func(_ context.Context, url string) ([]int, *http.Response, error) {
		switch url {
		case "":
			return []int{1, 2}, respWithNext("https://api.example.test/items?page=2"), nil
		case "https://api.example.test/items?page=2":
			return []int{3}, respWithNext(""), nil // no next link → stop
		default:
			return nil, nil, context.Canceled
		}
	}
	pager := pagination.NewLinkHeader(fetch)
	got := collect(t, pager.Items(context.Background()))

	want := []int{1, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("items = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("items = %v, want %v", got, want)
		}
	}
}

func TestNextLink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		link string
		want string
	}{
		{"single next", `<https://api/x?page=2>; rel="next"`, "https://api/x?page=2"},
		{"next and prev", `<https://api/p1>; rel="prev", <https://api/p3>; rel="next"`, "https://api/p3"},
		{"unquoted rel", `<https://api/n>; rel=next`, "https://api/n"},
		{"multiple rels", `<https://api/n>; rel="prev next"`, "https://api/n"},
		{"comma in url", `<https://api/x?a=1,2>; rel="next"`, "https://api/x?a=1,2"},
		{"no next", `<https://api/p>; rel="prev"`, ""},
		{"empty", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := http.Header{}
			if tc.link != "" {
				h.Set("Link", tc.link)
			}
			resp := &http.Response{Header: h}
			if got := pagination.NextLink(resp); got != tc.want {
				t.Fatalf("NextLink = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNextLinkNilResponse(t *testing.T) {
	t.Parallel()

	if got := pagination.NextLink(nil); got != "" {
		t.Fatalf("NextLink(nil) = %q, want empty", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pagination/ -run 'NewPageNumber|NewLinkHeader|NextLink' -v`
Expected: FAIL — `NewPageNumber`/`NewLinkHeader`/`NextLink` undefined.

- [ ] **Step 3: Create `pagination/strategies.go`**

```go
// pagination/strategies.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package pagination

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// NewPageNumber returns a Pager that fetches sequentially numbered pages starting
// at startPage, advancing until fetch returns a page with no items. fetch is
// called with the page number and returns that page's items.
func NewPageNumber[T any](startPage int, fetch func(ctx context.Context, page int) ([]T, error), opts ...Option) *Pager[T] {
	tokenFetch := func(ctx context.Context, token string) (Page[T], error) {
		page := startPage
		if token != "" {
			n, err := strconv.Atoi(token)
			if err != nil {
				return Page[T]{}, fmt.Errorf("pagination: invalid page token %q: %w", token, err)
			}
			page = n
		}
		items, err := fetch(ctx, page)
		if err != nil {
			return Page[T]{}, err
		}
		next := ""
		if len(items) > 0 {
			next = strconv.Itoa(page + 1)
		}
		return Page[T]{Items: items, NextToken: next}, nil
	}
	return New(tokenFetch, opts...)
}

// NewLinkHeader returns a Pager that follows RFC 8288 Link headers. fetch is
// called with the next URL (empty for the first page) and returns the page's
// items and the HTTP response whose Link header carries the next URL.
func NewLinkHeader[T any](fetch func(ctx context.Context, url string) ([]T, *http.Response, error), opts ...Option) *Pager[T] {
	tokenFetch := func(ctx context.Context, token string) (Page[T], error) {
		items, resp, err := fetch(ctx, token)
		if err != nil {
			return Page[T]{}, err
		}
		return Page[T]{Items: items, NextToken: NextLink(resp)}, nil
	}
	return New(tokenFetch, opts...)
}

// NextLink returns the URL of resp's RFC 8288 Link header entry whose rel
// includes "next", or "" when there is none (or resp is nil).
func NextLink(resp *http.Response) string {
	if resp == nil {
		return ""
	}
	for _, value := range resp.Header.Values("Link") {
		for _, entry := range splitLinkEntries(value) {
			url, rel := parseLinkEntry(entry)
			if url != "" && relHasNext(rel) {
				return url
			}
		}
	}
	return ""
}

// splitLinkEntries splits a Link header value on commas that are not inside the
// angle-bracketed URL of an entry.
func splitLinkEntries(value string) []string {
	var entries []string
	depth := 0
	start := 0
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				entries = append(entries, value[start:i])
				start = i + 1
			}
		}
	}
	entries = append(entries, value[start:])
	return entries
}

// parseLinkEntry extracts the <URL> and the rel parameter value from a single
// Link entry such as `<https://...>; rel="next"`.
func parseLinkEntry(entry string) (url, rel string) {
	entry = strings.TrimSpace(entry)
	open := strings.IndexByte(entry, '<')
	closeIdx := strings.IndexByte(entry, '>')
	if open != 0 || closeIdx < 0 {
		return "", ""
	}
	url = entry[open+1 : closeIdx]
	for _, param := range strings.Split(entry[closeIdx+1:], ";") {
		param = strings.TrimSpace(param)
		name, val, ok := strings.Cut(param, "=")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(name), "rel") {
			rel = strings.Trim(strings.TrimSpace(val), `"`)
		}
	}
	return url, rel
}

// relHasNext reports whether the space-separated rel value includes "next".
func relHasNext(rel string) bool {
	for _, token := range strings.Fields(rel) {
		if strings.EqualFold(token, "next") {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pagination/ -v`
Expected: PASS — all strategy/NextLink tests plus the Task 1 and pre-existing tests.

- [ ] **Step 5: Run the full suite**

Run: `go test ./...`
Expected: PASS across every package.

- [ ] **Step 6: Commit**

```bash
git add pagination/strategies.go pagination/strategies_test.go
git commit -m "feat(pagination): add page-number and Link-header strategies"
```

---

## Task 3: docs and full gate

**Files:**
- Modify: `pagination/doc.go`, `README.md`

- [ ] **Step 1: Extend the `pagination` package doc**

Read `pagination/doc.go`. The package comment currently describes token pagination
only. Extend it (keeping the single contiguous `//` block above `package pagination`,
no duplicate header) to mention the strategies, e.g. append a sentence:

```go
// Beyond the token/cursor strategy ([New]), [NewPageNumber] paginates by
// sequential page number and [NewLinkHeader] follows RFC 8288 Link headers
// (parsed by [NextLink]). [WithMaxPages] bounds iteration.
```

- [ ] **Step 2: Update `README.md`**

Read `README.md`. If the architecture/package table describes `pagination`, update
its description to mention the cursor/page-number/Link-header strategies and the
`WithMaxPages` cap. Keep the edit tight; match the table style.

- [ ] **Step 3: Run the full gate**

Run:
```bash
gofmt -l .
go vet ./...
go test -race ./...
```
Expected: `gofmt -l .` prints nothing; `go vet` clean; every package passes under the race detector.

- [ ] **Step 4: Commit**

```bash
git add pagination/doc.go README.md
git commit -m "docs: document pagination strategies and WithMaxPages"
```

---

## Self-Review notes (for the implementer)

- **Spec coverage:** cap (Task 1); page-number + Link-header + `NextLink` parser (Task 2); docs (Task 3). All spec sections map to a task.
- **Type consistency:** `Option`/`WithMaxPages`, `New[T](fetch, opts...)`, `NewPageNumber[T](startPage, fetch, opts...)`, `NewLinkHeader[T](fetch, opts...)`, `NextLink(resp) string` are used identically across tasks.
- **Backward compatibility:** `New` is variadic, so existing `New(fetch)` callers and tests are unaffected.
- **Link parsing:** splits on commas outside `<…>`; accepts quoted and unquoted `rel`, multiple space-separated rels; nil/empty → `""`.
- **`make check`** green before opening the PR.
