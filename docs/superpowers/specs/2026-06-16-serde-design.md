# Serde — design

**Date:** 2026-06-16
**Status:** Approved (standing delegation); ready for implementation planning
**Subsystem:** #5 of the Go SDK platform-parity roadmap

## Context

The `serde` package is a placeholder. Java and Python expose a serialization seam
(`Serde` = serializer + deserializer) with a default JSON implementation and a
`Tristate` type (Absent / Null / Present) for PATCH payloads. This subsystem brings
that to Go idiomatically and fulfills the deferred `httperr` "DecodeInto hook lands
when serde exists" commitment from the error-model spec.

## Decisions

1. **Byte-based seam.** `Marshaler`/`Unmarshaler` single-method interfaces over
   `[]byte`; a combined `Serde` embeds both. (Streaming Encoder/Decoder is out of
   scope — `encoding/json` already offers it directly.)
2. **Default is JSON.** `serde.JSON` is a `Serde` backed by `encoding/json`.
3. **`Tristate[T]` leans on `omitzero`.** The zero value is Absent; an Absent
   field tagged `json:",omitzero"` is omitted entirely (via `IsZero`), Null
   marshals to `null`, Present marshals the value. This is the modern Go (1.24+)
   idiom for PATCH tristate and the module targets Go 1.26.
4. **`httperr.ResponseError.DecodeInto(v any) error`** decodes the buffered error
   body as JSON via `serde.JSON`, honoring the earlier deferral.

## Architecture

### Seam and default (`serde` package, stdlib-only)

```go
// Marshaler encodes a value to bytes.
type Marshaler interface {
	Marshal(v any) ([]byte, error)
}

// Unmarshaler decodes bytes into v (a pointer).
type Unmarshaler interface {
	Unmarshal(data []byte, v any) error
}

// Serde is a paired Marshaler and Unmarshaler.
type Serde interface {
	Marshaler
	Unmarshaler
}

// JSON is the default Serde, backed by encoding/json.
var JSON Serde = jsonSerde{}

type jsonSerde struct{}

func (jsonSerde) Marshal(v any) ([]byte, error)      { return json.Marshal(v) }
func (jsonSerde) Unmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }
```

### `Tristate[T]` (PATCH semantics)

```go
// Tristate distinguishes a field that is absent (omitted), explicitly null, or
// present with a value — the three states a JSON PATCH payload must express. The
// zero value is Absent.
type Tristate[T any] struct {
	state tristate // absent | null | present
	value T
}

func Absent[T any]() Tristate[T]
func Null[T any]() Tristate[T]
func Present[T any](v T) Tristate[T]

func (t Tristate[T]) IsAbsent() bool
func (t Tristate[T]) IsNull() bool
func (t Tristate[T]) IsPresent() bool

// Get returns the value and whether it is present (a non-null, non-absent value).
func (t Tristate[T]) Get() (T, bool)

// IsZero reports whether the value is Absent, so a struct field tagged
// `json:",omitzero"` is omitted from the encoded output.
func (t Tristate[T]) IsZero() bool

// MarshalJSON encodes Present as the value and Null as "null". (Absent is
// normally omitted by omitzero before MarshalJSON is reached.)
func (t Tristate[T]) MarshalJSON() ([]byte, error)

// UnmarshalJSON decodes "null" as Null and any other value as Present. A field
// absent from the input is never passed here, so it remains Absent.
func (t *Tristate[T]) UnmarshalJSON(data []byte) error
```

**Usage:**
```go
type PatchUser struct {
	Name  serde.Tristate[string] `json:"name,omitzero"`
	Email serde.Tristate[string] `json:"email,omitzero"`
}
// Absent Name → omitted; Null Email → "email":null; Present → "name":"x".
```

### `httperr` integration

```go
// DecodeInto unmarshals the buffered error-response body into v as JSON. It
// returns an error when the body is empty or cannot be decoded. The body is the
// one captured by FromResponse (truncated to the internal cap).
func (e *ResponseError) DecodeInto(v any) error
```

Implemented with `serde.JSON.Unmarshal(e.body, v)`; returns a sentinel/wrapped
error when `e.body` is empty. `httperr` imports `serde` (acyclic: `serde` imports
only `encoding/json`).

## Edge cases

- `Tristate` zero value is Absent (`IsZero()==true`), so `omitzero` omits it; this
  is the whole point — a freshly-declared field is "not set".
- `UnmarshalJSON` of `"null"` sets Null and resets `value` to the zero `T`.
- A non-`"null"` payload that fails to decode into `T` returns the decode error
  and leaves the state unchanged-ish (caller should treat as error).
- `Get()` returns `(zero, false)` for Absent and Null; `(value, true)` for Present.
- `DecodeInto` on a `ResponseError` with no body → error (nothing to decode).
- `MarshalJSON` for Absent returns `"null"` for safety, but with `omitzero` the
  field is omitted before this path; without `omitzero`, an Absent field encodes
  as `null` (documented).

## Package layout

| Path | Change |
|---|---|
| `serde/doc.go` | replace placeholder comment |
| `serde/serde.go` (+ test) | `Marshaler`/`Unmarshaler`/`Serde`/`JSON` |
| `serde/tristate.go` (+ test) | `Tristate[T]` + constructors/accessors/JSON |
| `httperr/httperr.go` (+ test) | `ResponseError.DecodeInto` |
| `doc.go`, `README.md`, `CLAUDE.md` | document; de-placeholder `serde` |

## Testing

- `serde.JSON`: round-trips a struct (Marshal then Unmarshal); Unmarshal error on
  bad JSON.
- `Tristate`: constructors set the right state; `Get` semantics; `IsZero` true only
  for Absent; round-trip in a struct with `omitzero` — Absent omitted, Null →
  `"f":null`, Present → `"f":v`; UnmarshalJSON of `null` → Null, of a value →
  Present, of an absent field → stays Absent.
- `httperr.DecodeInto`: decodes a JSON error body into a struct; empty body →
  error.
- Table-driven, parallel; stdlib-only; `gofmt`/`go vet`/`go test -race` clean.

## Out of scope (deferred)

- Streaming `Encoder`/`Decoder` (use `encoding/json` directly).
- Non-JSON default Serdes (XML, protobuf) — users implement `Serde`.
- Wiring serde into pagination item decoding (separate concern).
- A `TristateModule`-style registry (not needed; `MarshalJSON`/`UnmarshalJSON`
  suffice).
