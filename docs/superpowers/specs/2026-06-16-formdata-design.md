# Multipart form-data body builder — design

**Date:** 2026-06-16
**Status:** Approved (standing delegation); ready for implementation planning
**Subsystem:** deferred-feature #1 (multipart bodies, from the HTTP-value-types roadmap item)

## Context

File uploads need a `multipart/form-data` body. Go's `mime/multipart` builds one
but the ergonomics (writer, boundary, content-type, retry-replayable body) are
fiddly to assemble correctly each time. This package wraps it into a small,
chainable builder that produces a replayable body and the matching `Content-Type`.

## Decisions

1. **Wrap `mime/multipart`.** Don't reinvent encoding; provide an ergonomic
   builder over `multipart.Writer`.
2. **Replayable body.** The body is buffered in memory and returned as a
   `*bytes.Reader`, so `http.NewRequest` sets `GetBody` automatically and the retry
   policy can replay it. (Consistent with the SDK's "buffer bodies that need to be
   retried" guidance.)
3. **Chainable, deferred-error builder.** `Field`/`File` return the `*Form` and
   accumulate the first error; `Build`/`NewRequest` surface it. No panics.
4. **A `NewRequest` convenience** that returns a ready `*http.Request` with the
   `Content-Type` (including boundary) already set.

## Architecture

### `formdata` package (stdlib + `header`)

```go
// Form builds a multipart/form-data request body. The zero value is not usable;
// create one with New. A Form is not safe for concurrent use.
type Form struct {
	buf bytes.Buffer
	w   *multipart.Writer
	err error
}

// New returns an empty Form.
func New() *Form

// Field adds a text field. It returns f for chaining.
func (f *Form) Field(name, value string) *Form

// File adds a file part read from r (filename sets the part's filename). It
// returns f for chaining.
func (f *Form) File(field, filename string, r io.Reader) *Form

// FileBytes adds a file part from an in-memory byte slice.
func (f *Form) FileBytes(field, filename string, data []byte) *Form

// ContentType returns the multipart Content-Type, including the boundary. It is
// valid immediately after New (the boundary is fixed at construction).
func (f *Form) ContentType() string

// Build finalizes the form and returns a replayable body. After Build no more
// parts may be added. It returns the first error encountered while building.
func (f *Form) Build() (*bytes.Reader, error)

// NewRequest builds the body and returns an *http.Request with the multipart
// Content-Type header set. It is the ergonomic entry point.
func (f *Form) NewRequest(ctx context.Context, method, url string) (*http.Request, error)
```

### Behaviour

- `New` creates a `multipart.Writer` over the internal buffer; the boundary is
  generated once.
- `Field` calls `w.WriteField`; `File` calls `w.CreateFormFile` then `io.Copy`
  from `r`; both no-op once `f.err` is set (first-error-wins).
- `Build` closes the writer (writes the trailing boundary) and returns
  `bytes.NewReader(f.buf.Bytes())`. Calling a builder method after `Build` is a
  programming error: `Build` sets a "closed" state and further `Field`/`File`
  record an error. (Implementation: a `built bool`; methods check it.)
- `NewRequest` = `Build` + `http.NewRequestWithContext` + set
  `header.ContentType` to `ContentType()`. Returns any build or request error.
- The returned body is a `*bytes.Reader`, so `http.NewRequest`/`NewRequestWithContext`
  populates `GetBody` and `ContentLength` — the retry policy can replay it.

## Edge cases

- An I/O error from `File`'s `io.Copy` (e.g. a failing reader) is captured and
  surfaced by `Build`/`NewRequest`.
- `Build` called twice → the second returns an error (already built); methods
  after `Build` record an error.
- An empty form (no parts) builds a valid (empty) multipart body.
- A `File` with an empty filename is allowed (mime/multipart handles it).
- `ContentType` is stable across the lifetime of the `Form` (boundary fixed at
  `New`), so calling it before or after `Build` returns the same value.

## Package layout

| Path | Change |
|---|---|
| `formdata/doc.go` (new) | package comment |
| `formdata/form.go` (new) | `Form` + methods |
| `formdata/form_test.go` (new) | build + parse-back + error tests |
| `doc.go`, `README.md`, `CLAUDE.md` | document; add the package |

## Testing

- Build a form with a field and a file; parse it back with `mime/multipart.Reader`
  (using the boundary from `ContentType`) and assert the field value and file
  contents/filename round-trip.
- `NewRequest` sets the `Content-Type` header (starts with `multipart/form-data;
  boundary=`) and produces a request whose body replays (read it twice via
  `GetBody`).
- A `File` reader that errors → `Build` returns that error.
- `Build` twice → second call errors; a `Field` after `Build` → `Build`/error
  state reflects it.
- Empty form builds without error.
- Table-driven where natural, parallel; stdlib + `header` only; `gofmt`/`go vet`/
  `go test -race` clean.

## Out of scope (deferred)

- Streaming (non-buffered) bodies for very large uploads (would not be
  retry-replayable; buffering is the documented default).
- Custom per-part headers/content types beyond what `CreateFormFile` sets (add a
  `FilePart(header textproto.MIMEHeader, ...)` later if needed).
