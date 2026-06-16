# HTTP value types — design

**Date:** 2026-06-16
**Status:** Approved (standing delegation); ready for implementation planning
**Subsystem:** #8 of the Go SDK platform-parity roadmap

## Context

Java/Python expose conditional-request value types (`ETag`, `HttpRange`,
`RequestConditions`) plus multipart bodies and JSONL/chunked stream helpers. This
subsystem delivers the cohesive **value-type** core — immutable types that shape a
request — and defers the builder/streamer helpers.

## Decisions

1. **Scope: conditional-request value types** — `ETag`, `Range`, and `Conditions`
   (If-Match / If-None-Match / If-Modified-Since / If-Unmodified-Since), in a new
   `conditions` package, each applied to a `*http.Request`.
2. **Immutable value types** with constructors and an `Apply(req)` that sets
   headers — consistent with `mediatype`. No client-wide policy or umbrella option
   (these are per-request).
3. **Add the missing header constants** (`Range`, `IfModifiedSince`,
   `IfUnmodifiedSince`) to the `header` package.
4. **Deferred (documented):** multipart/form-data body builder and JSONL/chunked
   stream helpers. Multipart is a body builder (different category) and JSONL is
   response-side streaming that overlaps with the SSE subsystem (#9). Each merits
   its own focused treatment.

## Architecture

### `conditions` package (stdlib-only)

```go
// ETag is an HTTP entity-tag validator (RFC 9110). The tag is the opaque value
// without surrounding quotes; weak marks a weak validator (W/).
type ETag struct {
	tag  string
	weak bool
}

// NewETag returns a strong entity tag.
func NewETag(tag string) ETag

// NewWeakETag returns a weak entity tag.
func NewWeakETag(tag string) ETag

// Parse parses an entity tag in wire form: "abc" or W/"abc".
func Parse(s string) (ETag, error)

func (e ETag) Tag() string    // the opaque tag (no quotes)
func (e ETag) Weak() bool
func (e ETag) String() string // wire form: "abc" or W/"abc"

// Range is an HTTP byte range for the Range header (RFC 9110 §14).
type Range struct {
	start  int64
	end    int64
	hasEnd bool
}

// Bytes returns the inclusive range [start, end].
func Bytes(start, end int64) Range

// BytesFrom returns the open-ended range [start, EOF).
func BytesFrom(start int64) Range

func (r Range) String() string                 // "bytes=start-end" or "bytes=start-"
func (r Range) Apply(req *http.Request)         // sets the Range header

// Conditions carries conditional-request headers (RFC 9110 §13). Empty slices and
// zero times are left unset.
type Conditions struct {
	IfMatch           []ETag
	IfNoneMatch       []ETag
	IfModifiedSince   time.Time
	IfUnmodifiedSince time.Time
}

// Apply sets the configured conditional headers on req. Each ETag list is
// comma-joined; times are formatted as HTTP-dates (RFC 1123 GMT). Unset fields
// leave the corresponding header untouched.
func (c Conditions) Apply(req *http.Request)
```

`ETag.String()` quotes the tag and prefixes `W/` for weak. `Parse` strips an
optional leading `W/`, then requires the remainder to be a quoted string,
returning a wrapped error otherwise. `Conditions.Apply` and `Range.Apply` set
headers via the `header` package constants and `http.TimeFormat`.

### `header` package additions

```go
IfModifiedSince   = "If-Modified-Since"
IfUnmodifiedSince = "If-Unmodified-Since"
Range             = "Range"
```

## Edge cases

- `Parse("")` and `Parse` of an unquoted/invalid value → error.
- `Parse(`W/"x"`)` → weak ETag with tag `x`; `Parse(`"x"`)` → strong.
- `ETag` with an empty tag is allowed (`""` → `""`); not validated beyond quoting.
- `Range` with `end < start` is formatted as given (caller's responsibility); no
  validation (matches the permissive HTTP value-type style).
- `Conditions.Apply` with all-zero fields is a no-op.
- `Conditions` with multiple ETags → comma-joined wire forms (`"a", "b"`).
- Zero `time.Time` fields are skipped; non-zero are formatted in UTC.
- `Apply` overwrites any existing value for the headers it sets (the caller asked
  for these conditions explicitly).

## Package layout

| Path | Change |
|---|---|
| `header/header.go` (modify) + existing test | add `Range`, `IfModifiedSince`, `IfUnmodifiedSince` |
| `conditions/doc.go`, `conditions/etag.go` (+ test) | `ETag` |
| `conditions/range.go` (+ test) | `Range` |
| `conditions/conditions.go` (+ test) | `Conditions` + `Apply` |
| `doc.go`, `README.md`, `CLAUDE.md` | document; add the new package |

## Testing

- `ETag`: `NewETag`/`NewWeakETag` String() wire forms; `Parse` of strong/weak/
  invalid/empty; round-trip `Parse(e.String()) == e`.
- `Range`: `Bytes(0,99).String() == "bytes=0-99"`; `BytesFrom(100).String() ==
  "bytes=100-"`; `Apply` sets the `Range` header.
- `Conditions.Apply`: sets `If-None-Match` from a list (comma-joined); sets
  `If-Modified-Since` as an HTTP-date; skips zero/empty fields; overwrites existing.
- `header`: the three new constants equal their canonical strings.
- Table-driven, parallel; stdlib-only; `gofmt`/`go vet`/`go test -race` clean.

## Out of scope (deferred)

- Multipart/form-data body builder (a future "request bodies" subsystem).
- JSONL/NDJSON and chunked-frame stream helpers (response-side streaming; overlaps
  with SSE, #9).
- `If-Range` and content-range parsing (response-side).
