# Pipeline completeness — design

**Date:** 2026-06-14
**Status:** Approved (design); ready for implementation planning
**Subsystem:** #1 of the Go SDK platform-parity roadmap

## Context

The Go SDK (`github.com/dexpace/go-sdk`) is the Go counterpart to the Java and
Python SDKs. Its core pipeline is already implemented and tested: a flat
`[]Policy` slice threaded through `Request.Next()`, with policy order fixed
positionally in `dexpace.New` (`user-agent → retry → logging → auth → custom →
transport`). Custom policies can only be appended at one spot (just before
transport).

The Java and Python SDKs expose a richer pipeline: a `Stage` ordering taxonomy
with "pillar" stages (one policy each) and surgical placement
(`insertAfter`/`insertBefore`/`replace`/`remove`), plus standard policies the Go
SDK lacks — redirect, idempotency-key, and set-date.

This subsystem brings the Go pipeline to enterprise parity **in capability**,
while staying within the Go SDK's defining constraints: lean on `net/http`, zero
third-party runtime dependencies, no builder types, functional options over
mutable builders.

## Decisions

These were settled during design and are not open for re-litigation in the plan:

1. **Ordering model:** adopt the Java pipeline *model* (stage ordering, pillar
   stages, before/after/replace/remove) but express it through Go's
   functional-options idiom — **not** a mutable builder. This honours the
   CLAUDE.md rule "No builder types."
2. **Redirects:** `net/http` owns them. No pipeline redirect policy. `*http.Client`
   already follows redirects (cap 10) and strips `Authorization`/`Cookie` on
   cross-origin redirects. We expose thin transport-level configuration instead.
3. **Idempotency-key:** default-on, **POST only**, header `Idempotency-Key`,
   keys are UUIDv4 from `crypto/rand`. Set only when the caller hasn't supplied
   one. Disable via `dexpace.WithoutIdempotency()`.
4. **Set-date:** included but **opt-in** via `dexpace.WithDate()`.
5. **Stage order (outermost → innermost):**
   `ClientIdentity → Idempotency → Retry → Auth → Date → Logging → transport`.
   Retry wraps auth (so a 401-triggered refresh works); logging is innermost so
   it records the actual on-the-wire request and measures the transport
   round-trip.

## Architecture

### The `Stage` model (assembly-time ordering layer)

The pipeline *engine* is unchanged: `Request.Next()` still walks a flat
`[]Policy`. The `Stage` model is a pure assembly-time layer that resolves a set
of placements into that flat slice once, at construction.

```go
// pipeline/stage.go
type Stage int

const (
	StageClientIdentity Stage = iota + 1 // user-agent, outermost
	StageIdempotency
	StageRetry
	StageAuth
	StageDate
	StageLogging // innermost; logs the on-the-wire request
)

// Placement pairs a Policy with where it belongs in stage order.
// Fields are unexported; construct with At/Before/After.
type Placement struct {
	stage  Stage
	offset int8 // -1 before, 0 at (pillar), +1 after
	policy Policy
}

func At(s Stage, p Policy) Placement     // pillar — "at" the stage
func Before(s Stage, p Policy) Placement // insertBefore
func After(s Stage, p Policy) Placement  // insertAfter

// NewStaged resolves placements into ordered policies and builds a Pipeline.
func NewStaged(t Transporter, ps ...Placement) Pipeline
```

**Resolution algorithm:** each placement gets a sort key derived from
`(stage, offset)` — `Before` sorts just under the stage, `At` at it, `After`
just over it. A **stable** sort flattens placements into `[]Policy`; ties
(same stage+offset) preserve insertion order. Resolution runs once at
construction, is allocation-light, and produces a deterministic order.

- **Replace a pillar:** provide another `At(sameStage, …)`; last one wins.
- **Remove a built-in:** an umbrella `Without…` option omits its placement.

The existing `pipeline.New(t, ...Policy)` is retained unchanged for the simple
positional case and for users assembling pipelines by hand.

### New and changed policies

**`idempotency` package (new).** Earns its own package — it carries real logic
and interacts with retry.

```go
// idempotency/policy.go
type Options struct {
	Methods []string      // default ["POST"]
	Header  string        // default "Idempotency-Key"
	NewKey  func() string // default: UUIDv4 via crypto/rand
}

func NewPolicy(opts Options) *Policy
```

- Anchored at `StageIdempotency` (outside retry → key minted once, stable across
  attempts).
- Acts only when the method matches **and** the header is absent
  (caller-supplied keys win).
- Generates a canonical UUIDv4 from `crypto/rand`; the 16 bytes are formatted
  in-package (zero third-party deps).
- Marks the request retry-safe via `pipeline.MarkIdempotent(req)`, so the retry
  policy can recognise an idempotent POST.

**`retry` package (enhanced — also a correctness fix).** The current
`retryableErr` retries *every* non-context transport error regardless of method,
despite a doc comment claiming method-awareness — so today it unsafely retries
non-idempotent POSTs. The fix introduces real method-awareness: on a transport
error, retry only when the method is safe/idempotent (GET, HEAD, OPTIONS, TRACE,
PUT, DELETE) **or** the request is marked idempotent. A request is marked
idempotent when the idempotency policy ran (`pipeline.IsIdempotent`) or when a
canonical `Idempotency-Key` header is present. This is both the correctness fix
and the synergy that makes default-on idempotency pay off: safe `POST` retries.
The body-replay guard is unaffected — a non-replayable body still blocks the
retry regardless of the key (see Edge cases).

**`pipeline` idempotency coordination.** To let the idempotency and retry
policies cooperate without an import cycle or an exported magic key, `pipeline`
gains two small functions: `MarkIdempotent(r *Request)` and
`IsIdempotent(r *Request) bool`, backed by an unexported request value. The
idempotency policy marks; the retry policy reads.

**set-date (no package).** A one-line policy; ship it inline in the umbrella,
exposed via `dexpace.WithDate()`. Anchored at `StageDate`; stamps the `Date`
header in RFC 1123 form (`http.TimeFormat`) when absent. Opt-in.

**client-identity (user-agent).** Stays the inline policy it is today, formally
anchored at `StageClientIdentity`.

### Transport redirect configuration

`net/http` owns redirects. `transport` exposes thin configuration that sets
`http.Client.CheckRedirect`:

```go
transport.New(
	transport.WithMaxRedirects(10),   // caps redirect hops
	transport.WithRedirectPolicy(fn), // full custom http.Client.CheckRedirect
)
```

There is **no** `StageRedirect` and no redirect policy in the pipeline. A
redirected call is one round-trip from the pipeline's perspective; per-hop
logging/tracing is intentionally not provided (the cost accepted for leaning on
`net/http`).

### Umbrella wiring (`options.go` / `client.go`)

```go
dexpace.New(
	dexpace.WithRetry(retry.Options{MaxRetries: 4}),
	dexpace.WithCredential(cred, "scope"),
	dexpace.WithDate(),                                // opt-in set-date
	dexpace.WithoutIdempotency(),                      // disable the default
	dexpace.WithPolicyBefore(pipeline.StageRetry, cb), // enterprise insertion
	dexpace.WithPolicyAfter(pipeline.StageAuth, signer),
)
```

`New` assembles the built-ins as `Placement`s at their stages (idempotency
default-on; set-date, auth, and logging conditional on their options), appends
the `WithPolicyBefore`/`WithPolicyAfter` placements, and calls
`pipeline.NewStaged`. The fixed positional list in today's `client.go` is
replaced by stage resolution.

New options: `WithDate()`, `WithoutIdempotency()`, `WithIdempotency(opts)`,
`WithPolicyBefore(stage, policy)`, `WithPolicyAfter(stage, policy)`.

## Behavior changes (call out in release notes)

- **Logging moves innermost.** Logs now show the request *after* auth/date
  stamping (still redacted), and latency measures the transport round-trip
  rather than the whole policy stack. More accurate; differs from today.
- **Idempotency-key is sent on POST by default.** New header on the wire for
  POST requests. Harmless to servers that ignore it; disable with
  `WithoutIdempotency()`.
- **Transport-error retries are now method-aware.** POST/PATCH *without* an
  idempotency marker are no longer retried on transport errors (previously they
  were, unsafely); POST *with* an idempotency key now is. Safe/idempotent methods
  are unchanged.

## Edge cases

- **Duplicate / conflicting placements:** stable ordering; insertion order breaks
  ties; `At` on the same stage twice means last wins (the "replace" semantic).
  Documented in `pipeline/doc.go`.
- **Streaming (non-replayable) POST body:** the idempotency key is still set, but
  retry still refuses to replay when `RewindBody` cannot rewind the body. The key
  does not override the body-replay guard.
- **`crypto/rand` read failure:** the idempotency policy returns a wrapped error;
  it never emits a weak or empty key and never panics.
- **Determinism & cost:** resolution happens once at construction, not per
  request; the per-request hot path is the existing flat-slice walk.

## Package layout

| Path | Change |
|---|---|
| `pipeline/stage.go` (+ `stage_test.go`) | new — `Stage`, `Placement`, `At/Before/After`, `NewStaged`, resolver |
| `pipeline/doc.go` | document the stage model and resolution semantics |
| `idempotency/doc.go`, `idempotency/policy.go` (+ test) | new package |
| `retry/policy.go` (+ tests) | idempotency-key retry-safety check |
| `transport/*.go` (+ tests) | `WithMaxRedirects`, `WithRedirectPolicy` |
| `client.go`, `options.go` (+ tests) | stage-based assembly; new options |

## Testing

- Table-driven, parallel (`t.Parallel()`); response bodies closed via
  `t.Cleanup`; fakes are local `transporterFunc` / credential stubs.
- Resolver: assert the final flat policy order for representative placement sets
  (pillars, before/after, replace, ties).
- Idempotency: deterministic `NewKey` injection; assert header set only on
  matching method and only when absent; assert `crypto/rand` failure path.
- Retry: keyed-POST-on-transport-error retries; non-replayable keyed POST still
  blocked; idempotent methods unchanged.
- No new third-party dependencies; `gofumpt`/`goimports`/`go vet`/
  `golangci-lint` clean.

## Out of scope (deferred to later roadmap tiers)

- Redirect policy (net/http owns it).
- Challenge handlers, Basic, and Key credentials (Tier 3, auth breadth).
- Tracing/metrics policy and URL redactor (Tier 2, observability).
- Serde, config, SSE, webhooks, pagination breadth, HTTP value types.
