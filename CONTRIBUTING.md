# Contributing to the dexpace Go SDK

Thanks for helping build the SDK. This document covers the essentials; the full,
enforced rule set lives in [`CLAUDE.md`](./CLAUDE.md) and the dexpace
[Go styleguide](https://github.com/dexpace/styleguide/tree/main/go).

## Ground rules

- **Target Go 1.26+** and prefer modern idioms (generics, range-over-func
  iterators, `log/slog`, `math/rand/v2`, `min`/`max`).
- **No third-party runtime dependencies.** Non-test code imports only the
  standard library. If you need an external library, hide it behind an interface
  and keep it in tests or a future adapter package.
- **Lean on `net/http`.** Do not reinvent request/response/header models or add
  an I/O abstraction layer.
- **Every `.go` file** starts with the MIT license header (see any existing
  file) and belongs to a package with a doc comment.

## Before you open a PR

Run the full local gate:

```bash
make check        # go mod tidy + gofumpt + go vet + golangci-lint + go test -race
```

or individually:

```bash
go test -race -covermode=atomic ./...
go vet ./...
golangci-lint run
gofumpt -l .       # should print nothing
```

CI (`.github/workflows/ci.yml`) runs the same checks on Go 1.26 and fails on any
diff from `go mod tidy`, any vet/lint finding, or any test failure.

## Tests

- Table-driven where it helps; call `t.Parallel()` in every test.
- Close response bodies with `t.Cleanup(func() { _ = resp.Body.Close() })`.
- Use local fakes (a `transporterFunc`, a stub `TokenCredential`) — no mocking
  frameworks.
- Add an `Example` for user-facing API where it aids the docs.

## Commits

Conventional prefixes, imperative subject ≤ 72 chars:

```
feat: add SSE event reader
fix: rewind body before each retry attempt
docs: document pipeline ordering
test: cover Retry-After parsing
chore: bump golangci-lint config
```

No `Co-Authored-By` trailers; commits author as the repository owner.
