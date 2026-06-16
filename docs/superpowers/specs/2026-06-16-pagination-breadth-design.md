# Pagination breadth — design

**Date:** 2026-06-16
**Status:** Approved (standing delegation); ready for implementation planning
**Subsystem:** #7 of the Go SDK platform-parity roadmap

## Context

The `pagination` package provides a generic, transport-agnostic `Pager[T]` over
`iter.Seq2`, driven by a `FetchFunc[T](ctx, token) (Page[T], error)` where
`Page.NextToken == ""` ends iteration. This token model already covers
cursor/continuation-token pagination. Java/Python additionally ship page-number
and Link-header strategies and a `maxPages` cap.

## Decisions

1. **Reuse the token core.** The `token` is opaque, so page-number and
   Link-header strategies are thin constructors that compute `NextToken` and
   delegate to the existing `Pager[T]`. No new iterator type.
2. **Add a bounded-iteration cap.** `WithMaxPages(n)` is a functional option on
   all constructors (honoring "bounded everything"; the pager is currently
   unbounded). `New` becomes variadic — backward compatible.
3. **Ship an RFC 8288 `Link` parser.** `NextLink(resp)` extracts the `rel="next"`
   URL; it is both used by `NewLinkHeader` and exported for direct use.
4. **Cursor/token strategy stays `New`.** Page-number and Link-header get their
   own constructors.

## Architecture

### Options and the cap

```go
// Option configures a Pager.
type Option func(*pagerConfig)

// WithMaxPages caps how many pages a Pager yields. A value <= 0 means unlimited.
func WithMaxPages(n int) Option
```

`Pager[T]` gains an unexported `maxPages int` (0 = unlimited). `Pages` counts
yielded pages and stops once the cap is reached. `New` and the new constructors
take `opts ...Option`.

### Existing `New` (cursor/token strategy), now variadic

```go
func New[T any](fetch FetchFunc[T], opts ...Option) *Pager[T]
```

Existing callers (`New(fetch)`) are unaffected.

### Page-number strategy

```go
// NewPageNumber returns a Pager that fetches sequentially numbered pages starting
// at startPage, advancing until fetch returns a page with no items. fetch is
// called with the page number and returns that page's items.
func NewPageNumber[T any](startPage int, fetch func(ctx context.Context, page int) ([]T, error), opts ...Option) *Pager[T]
```

It wraps `fetch` into a `FetchFunc[T]` where the opaque token encodes the next
page number (empty token → `startPage`). After fetching page *P*: `NextToken` is
`strconv.Itoa(P+1)` when the page has items, or `""` (stop) when empty. A
non-numeric token (which the SDK never produces) yields a wrapped error.

### Link-header strategy

```go
// NewLinkHeader returns a Pager that follows RFC 8288 Link headers. fetch is
// called with the next URL (empty for the first page) and returns the page's
// items and the HTTP response whose Link header carries the next URL.
func NewLinkHeader[T any](fetch func(ctx context.Context, url string) ([]T, *http.Response, error), opts ...Option) *Pager[T]

// NextLink returns the URL of the resp's RFC 8288 Link header entry whose rel
// includes "next", or "" when there is none (or resp is nil).
func NextLink(resp *http.Response) string
```

`NewLinkHeader` wraps `fetch` into a `FetchFunc[T]` whose `NextToken` is
`NextLink(resp)` (empty → stop).

### `Link` header parsing

`NextLink` reads every `Link` header value (`resp.Header.Values("Link")`), splits
entries on commas that are **not** inside `<…>`, and for each `<URL>; param=value…`
entry checks whether any `rel` parameter's space-separated tokens include `next`
(case-insensitive; `rel="next"` and `rel=next` both accepted). The first matching
entry's URL is returned.

## Edge cases

- `WithMaxPages(0)` / negative → unlimited (documented).
- Page-number: an empty page stops iteration even if the API would serve more; a
  page with items but a future empty page stops there. Callers needing a total-count
  stop use the cursor strategy with their own `NextToken`.
- `NextLink(nil)` → `""`; no `Link` header → `""`; `Link` present but no
  `rel="next"` → `""`.
- A `Link` URL containing a comma inside `<…>` is not split mid-URL.
- The cap is enforced in `Pages`, so `Items` (built on `Pages`) honors it too.
- Early `break` from the iterator stops fetching (existing behavior preserved).

## Package layout

| Path | Change |
|---|---|
| `pagination/pagination.go` (modify) | `Option`, `WithMaxPages`, variadic `New`, cap in `Pages` |
| `pagination/strategies.go` (new) | `NewPageNumber`, `NewLinkHeader`, `NextLink` |
| `pagination/pagination_test.go` (modify) | cap test |
| `pagination/strategies_test.go` (new) | page-number, Link-header, NextLink parser tests |

## Testing

- Cap: a fetch that always returns a non-empty `NextToken` yields exactly `n`
  pages with `WithMaxPages(n)`; `Items` honors the cap.
- Page-number: three numbered pages then an empty page → flattened items in order;
  the empty page stops iteration; `startPage` honored.
- Link-header: pages chained by `Link: <url>; rel="next"` until a response with no
  next link; items flattened in order.
- `NextLink`: single next; multiple entries (next + prev); `rel=next` unquoted;
  multiple space-separated rels (`rel="prev next"`); a URL containing a comma; no
  Link header; nil response.
- Existing pagination tests still pass (`New` variadic, default unlimited).
- Table-driven, parallel; stdlib-only; `gofmt`/`go vet`/`go test -race` clean.

## Out of scope (deferred)

- A pluggable `PaginationStrategy` interface (the constructors cover the common
  schemes; a generic strategy interface adds surface without clear need).
- Async/parallel page prefetching.
- Offset/limit as a distinct constructor (it is page-number with arithmetic the
  caller already controls).
