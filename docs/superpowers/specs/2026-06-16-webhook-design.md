# Webhook signature verification — design

**Date:** 2026-06-16
**Status:** Approved (standing delegation); ready for implementation planning
**Subsystem:** #10 (final) of the Go SDK platform-parity roadmap

## Context

The `webhook` package is a placeholder. Java/Python verify inbound webhook
signatures with constant-time HMAC comparison and a timestamp-tolerance window to
defeat replay. This subsystem brings that to Go.

## Decisions

1. **HMAC-SHA256, constant-time.** Verification computes `HMAC-SHA256(secret,
   payload)` and compares against the supplied signature with `hmac.Equal`
   (constant-time over the raw MAC bytes, not the hex strings).
2. **Two entry points.** `Verifier.Verify` for an explicit signed payload, and
   `Verifier.VerifyTimestamp` for the common Stripe-style scheme (signed payload =
   `"<unix>.<body>"`) with a tolerance window.
3. **Injected `now`.** `VerifyTimestamp` takes `now time.Time` rather than calling
   `time.Now`, for deterministic tests and caller control.
4. **Typed sentinel errors** for `errors.Is`: signature mismatch and
   timestamp-outside-tolerance.
5. **A `Sign` helper** (hex HMAC-SHA256) for symmetry/testing and for callers who
   send signed payloads.

## Architecture

### `webhook` package (stdlib-only)

```go
// ErrSignatureMismatch is returned when a signature does not match the payload.
var ErrSignatureMismatch = errors.New("webhook: signature mismatch")

// ErrTimestampOutsideTolerance is returned when a timestamped payload is outside
// the verifier's tolerance window.
var ErrTimestampOutsideTolerance = errors.New("webhook: timestamp outside tolerance")

// Sign returns the lowercase hex-encoded HMAC-SHA256 of payload keyed by secret.
func Sign(secret, payload []byte) string

// Verifier verifies HMAC-SHA256 webhook signatures. The zero value is not usable;
// build one with NewVerifier. It is safe for concurrent use.
type Verifier struct {
	secret    []byte
	tolerance time.Duration
}

// Option configures a Verifier.
type Option func(*Verifier)

// WithTolerance sets the allowed clock skew for VerifyTimestamp. The default is
// five minutes; a value <= 0 disables the timestamp check.
func WithTolerance(d time.Duration) Option

// NewVerifier returns a Verifier keyed by secret.
func NewVerifier(secret []byte, opts ...Option) *Verifier

// Verify reports whether sigHex is a valid lowercase/uppercase hex HMAC-SHA256
// signature of payload, compared in constant time. It returns nil on a match and
// ErrSignatureMismatch otherwise (including when sigHex is not valid hex).
func (v *Verifier) Verify(payload []byte, sigHex string) error

// VerifyTimestamp implements the common scheme: the signed payload is the Unix
// timestamp, a ".", and the body. It first checks that timestamp is within the
// configured tolerance of now (unless tolerance <= 0), then verifies sigHex
// against "<unix>." + body.
func (v *Verifier) VerifyTimestamp(body []byte, timestamp, now time.Time, sigHex string) error
```

### Verification detail

- `Verify`: `expected := hmacSHA256(secret, payload)` (raw bytes);
  `provided, err := hex.DecodeString(sigHex)`; if `err != nil` → `ErrSignatureMismatch`;
  if `!hmac.Equal(provided, expected)` → `ErrSignatureMismatch`; else nil.
  `hmac.Equal` is constant-time and length-safe.
- `VerifyTimestamp`: if `tolerance > 0` and `abs(now.Sub(timestamp)) > tolerance`
  → `ErrTimestampOutsideTolerance`; build `signed := strconv.FormatInt(timestamp.Unix(),10) + "." + string(body)`;
  return `Verify([]byte(signed), sigHex)`.
- `Sign`: `hex.EncodeToString(hmacSHA256(secret, payload))`.

## Edge cases

- Invalid hex in `sigHex` → `ErrSignatureMismatch` (never panics, no information
  leak beyond "mismatch").
- Wrong-length signature → `hmac.Equal` returns false → `ErrSignatureMismatch`.
- `WithTolerance(0)` (or negative) → the timestamp window is skipped; only the
  signature is checked.
- `now` before or after `timestamp`: the check uses the absolute difference, so
  both a stale event and a future-dated one outside the window are rejected.
- Empty secret/payload are allowed (HMAC of empty inputs is well-defined); not
  validated.
- `Sign` and `Verify` round-trip: `verify(payload, Sign(secret, payload)) == nil`.

## Package layout

| Path | Change |
|---|---|
| `webhook/doc.go` (modify) | real package comment |
| `webhook/webhook.go` (new) | `Sign`, `Verifier`, `Verify`, `VerifyTimestamp`, errors, options |
| `webhook/webhook_test.go` (new) | round-trip, mismatch, bad-hex, tolerance tests |
| `doc.go`, `README.md`, `CLAUDE.md` | document; de-placeholder `webhook` |

## Testing

- `Sign`/`Verify` round-trip succeeds; a tampered payload → `ErrSignatureMismatch`.
- A wrong secret → `ErrSignatureMismatch`.
- Invalid hex signature → `ErrSignatureMismatch` (no panic).
- `VerifyTimestamp`: within tolerance → nil; outside (stale and future) →
  `ErrTimestampOutsideTolerance`; `WithTolerance(0)` skips the window; the signed
  payload format `"<unix>.<body>"` is what `Sign` must produce for a match.
- Constant-time path uses `hmac.Equal` (asserted indirectly via correctness; the
  property itself is a stdlib guarantee).
- Table-driven, parallel; stdlib-only; `gofmt`/`go vet`/`go test -race` clean.

## Out of scope (deferred)

- Provider-specific header parsing (e.g. Stripe's `t=…,v1=…` format). Callers parse
  their provider's header and pass the timestamp + signature to `VerifyTimestamp`;
  a thin parser can be added per provider later if needed.
- Signature schemes other than HMAC-SHA256 (e.g. Ed25519). Add when a concrete
  need arises.
