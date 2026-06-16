# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with
code in this repository.

## Repository

The Go counterpart to [`dexpace/java-sdk`](https://github.com/dexpace/java-sdk)
and [`dexpace/python-sdk`](https://github.com/dexpace/python-sdk). The
architecture is the same shape — a transport-agnostic HTTP-client toolkit with a
policy pipeline, auth, errors, pagination — but the public API uses Go idioms.

The defining decision of this port: **lean on `net/http`.** Requests and
responses are `*http.Request` / `*http.Response`; the transport seam is a
single-method `Transporter` interface that `*http.Client` satisfies; bodies are
plain `io.Reader` / `io.ReadCloser`. The Java SDK's immutable `Request`/`Headers`
model and its Okio-style `IoProvider` seam intentionally do **not** exist here —
the standard library is the contract, exactly as the Python port dropped the I/O
seam because `bytes`/`io` already covered it. Do not reintroduce a reinvented
HTTP model or an I/O abstraction layer.

It is a **single Go module** (`github.com/dexpace/go-sdk`) — not the multi-module
layout of the Java (9 Gradle modules) or Python (5 distributions) ports. The
umbrella `dexpace` package holds the `Client`; each toolkit layer is its own
package.

## Conventions (enforced — match these when adding code)

- **Go 1.26+.** Use modern idioms where they fit: generics, range-over-func
  iterators (`iter.Seq` / `iter.Seq2`), `math/rand/v2`, `log/slog`, the
  `min`/`max`/`clear` builtins, `errors.Is`/`As`/`Join`.
- **Follow the dexpace Go styleguide** (`../styleguide/go`), which defers to the
  [Google Go Style Guide](https://google.github.io/styleguide/go/). Priorities:
  correctness > performance > developer experience. Clarity, simplicity,
  concision, then consistency.
- **Zero third-party runtime dependencies.** Non-test code imports only the
  standard library. A third-party need is modelled behind an interface and lives
  in a `_test.go` or a future adapter, never in core.
- **Accept interfaces, return structs.** Interfaces are the narrowest possible
  and defined by the consumer (e.g. `pipeline.Transporter` is one method). No
  builder types — Go's struct literals, functional options, and `With*` copy
  helpers replace the Java/Kotlin builders.
- **Immutable by default.** Value types (`mediatype.MediaType`) are copied, not
  mutated; `With*` methods return new values. Exported fields are fine when the
  zero value is meaningful; reach for an unexported field + accessor only to
  protect an invariant (as `mediatype` does with its parameter map).
- **Errors are values — wrap with `%w`, never discard.** Return typed errors
  (`*httperr.ResponseError`) and sentinels for `errors.Is`; never `_`-drop an
  error that matters. Panic only for unrecoverable programmer errors at
  construction (e.g. `pipeline.New(nil)`).
- **`context.Context` is the first parameter** of any blocking/IO call and is
  honoured (the retry policy's `sleep` selects on `ctx.Done()`). No custom
  context types.
- **Bounded everything.** Loops, retries, and buffered reads have explicit caps
  (`retry.Options.MaxRetries`, `drainLimit`, `maxErrorBodyBytes`).
- **Package = directory, named for what it provides** so it composes at the call
  site (`retry.NewPolicy`, `auth.NewBearerTokenPolicy`). No `utils`/`common`/
  `helpers`/`base`.
- **One file per major type**; `doc.go` carries the package comment for
  multi-file packages. Every package has a Go doc comment.
- **MIT license header on every `.go` file** (src and tests), before the package
  clause:

  ```go
  // Copyright (c) 2026 dexpace and Omar Aljarrah.
  // Licensed under the MIT License. See LICENSE in the repository root for details.
  ```

- **`gofumpt` + `goimports` clean; `go vet` and `golangci-lint` clean**
  (`.golangci.yml`). Import groups: stdlib, then `github.com/dexpace/go-sdk/...`.
- **Table-driven, parallel tests** (`t.Parallel()`); response bodies closed via
  `t.Cleanup`. Fakes are local `transporterFunc` / credential stubs, not mocking
  frameworks.
- **Commit style:** `feat:` / `fix:` / `chore:` / `docs:` / `test:` prefixes.

## Repository Layout

```
go-sdk/
├── go.mod                     # module github.com/dexpace/go-sdk, go 1.26
├── doc.go, version.go         # package dexpace (umbrella): SDK doc + Version
├── client.go, options.go      # Client, New, functional Options
├── example_test.go            # runnable Example
├── pipeline/                  # Policy, PolicyFunc, Transporter, Request, Pipeline
├── transport/                 # default net/http Transporter
├── retry/                     # retry policy (backoff + jitter + Retry-After)
├── auth/                      # TokenCredential, BearerTokenPolicy, StaticToken
├── logging/                   # slog request/response policy
├── httperr/                   # ResponseError + FromResponse
├── mediatype/                 # immutable MediaType + constants
├── header/                    # canonical header-name constants
├── pagination/                # generic iter.Seq2 Pager
├── redact/                    # default-deny URL redactor (userinfo + query values)
├── instrumentation/           # tracing + metrics SPIs, no-op defaults, policies
├── config/                    # layered override→env→default settings
├── sse/  webhook/  serde/     # placeholders (doc.go only)
├── .golangci.yml  Makefile  .github/workflows/ci.yml
└── CONTRIBUTING.md  CLAUDE.md  README.md  LICENSE
```

### Common commands (from the repository root)

```bash
make check                 # tidy + fmt + vet + lint + test
go test -race ./...        # full test suite with the race detector
go vet ./...
golangci-lint run
```

## Architecture — Big Picture

A `Pipeline` runs an ordered chain of `Policy` values over an `*http.Request`,
terminating in a `Transporter`:

1. **`pipeline`** — `Policy.Do(*Request)` inspects/mutates the request and calls
   `Request.Next()` to continue (or returns early to short-circuit). `Next` may
   be called repeatedly; `Request.RewindBody()` (backed by `http.Request.GetBody`)
   replays the body so the retry policy can re-send. The terminal transport
   policy is appended by `pipeline.New`.
2. **`transport`** — wraps an `*http.Client` (cloned `http.DefaultTransport`
   with larger idle-conn limits) to satisfy `Transporter`.
3. **Policies** — `retry`, `auth`, `logging`, each a `Policy`. Order is set by
   `dexpace.New`: `user-agent → idempotency → retry → auth → date → [tracing] → [metrics] → logging → custom → transport`.
4. **Value layer** — `mediatype`, `header`, `httperr`, `pagination`: small,
   stdlib-only helpers over `net/http`.

## Things That Will Bite You

- **`Request.RewindBody` needs `http.Request.GetBody`.** `net/http` sets it
  automatically for `bytes.Reader`/`bytes.Buffer`/`strings.Reader` bodies. A
  streaming body (`io.Reader` with no `GetBody`) is **not** replayable — rewind
  returns an error and retries fail. Buffer such bodies before sending.
- **`BearerTokenPolicy` is HTTPS-only.** It returns `auth.ErrInsecureTransport`
  for a non-`https` URL rather than leaking a token. Tests must use `https://`
  URLs (a stub transporter never dials).
- **Policy order changes semantics.** Retry is outside auth, so a 401-triggered
  token refresh requires the auth policy to be inside retry (it is, by default).
  Moving logging outside retry collapses per-attempt logs into one.
- **`httperr.FromResponse` consumes and rewinds the body.** It buffers up to
  `maxErrorBodyBytes` and replaces `resp.Body` with an in-memory reader; bytes
  beyond the cap are dropped from the rewound body.
- **No Go toolchain runs in some sandboxes.** When you cannot `go build`/`go
  test` locally, write to compile by inspection (stdlib APIs only) and rely on CI
  (`.github/workflows/ci.yml`) as the gate.
