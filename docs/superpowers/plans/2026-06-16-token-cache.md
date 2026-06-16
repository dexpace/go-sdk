# Pluggable Token Cache Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `BearerTokenPolicy`'s token cache pluggable: add an `auth.TokenCache` interface with an `InMemoryTokenCache` default, refactor the policy to use it (behaviour preserved), and add a `dexpace.WithTokenCache` umbrella option for sharing tokens across clients.

**Architecture:** `BearerTokenPolicy` stores tokens in a `TokenCache` keyed by the scope set; `NewBearerTokenPolicy` keeps a private in-memory cache (unchanged behaviour), while `NewBearerTokenPolicyWithCache` injects a shared one. The per-policy refresh lock and freshness window are unchanged.

**Tech Stack:** Go 1.26+, standard library only (`strings`, `sync`, `time`, `net/http`). Zero third-party dependencies.

**Conventions every task must follow:**
- MIT license header on every `.go` file before the `package` clause.
- Import groups: stdlib, blank line, then `github.com/dexpace/go-sdk/...`.
- Tests use `t.Parallel()`; stdlib-only; bodies closed via `t.Cleanup`/`Close`. Bearer tests use `https://` URLs.
- Tools: Go 1.26.3; `gofumpt`/`golangci-lint` NOT installed — use `gofmt`, `go vet`, `go test -race`.
- Run commands from the repo root `/Users/omar/dexpace/go-sdk`.

---

## File Structure

| Path | Responsibility |
|---|---|
| `auth/cache.go` (new) + test | `TokenCache`, `InMemoryTokenCache` |
| `auth/bearer.go` (modify) | use `TokenCache`; add `NewBearerTokenPolicyWithCache`; `fresh` helper |
| `auth/bearer_test.go` (modify) | shared-cache test |
| `options.go`, `client.go` (modify) | `WithTokenCache` + wiring |
| `client_test.go` (modify) | shared-cache-across-clients test |
| `doc.go`, `README.md` (modify) | document |

---

## Task 1: `TokenCache` + `InMemoryTokenCache` + bearer refactor

**Files:**
- Create: `auth/cache.go`, `auth/cache_test.go`
- Modify: `auth/bearer.go`, `auth/bearer_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// auth/cache_test.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package auth_test

import (
	"sync"
	"testing"

	"github.com/dexpace/go-sdk/auth"
)

func TestInMemoryTokenCacheGetSet(t *testing.T) {
	t.Parallel()

	c := auth.NewInMemoryTokenCache()
	if _, ok := c.Get("k"); ok {
		t.Fatal("missing key should report not found")
	}
	c.Set("k", auth.AccessToken{Token: "t"})
	got, ok := c.Get("k")
	if !ok || got.Token != "t" {
		t.Fatalf("Get = (%v, %v), want token t / true", got, ok)
	}
}

func TestInMemoryTokenCacheConcurrent(t *testing.T) {
	t.Parallel()

	c := auth.NewInMemoryTokenCache()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Set("k", auth.AccessToken{Token: "t"})
			_, _ = c.Get("k")
		}()
	}
	wg.Wait()
}
```

Append to `auth/bearer_test.go` (it already imports `context`, `errors`, `io`, `net/http`, `strings`, `testing`, `time`, `auth`, `header`, `pipeline` and defines `transporterFunc` and `countingCredential`):

```go
func TestBearerSharedCacheReusesToken(t *testing.T) {
	t.Parallel()

	cred := &countingCredential{token: "tok", exp: time.Now().Add(time.Hour)}
	cache := auth.NewInMemoryTokenCache()

	run := func(p *auth.BearerTokenPolicy) {
		transport := transporterFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
		})
		pl := pipeline.New(transport, p)
		req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
		resp, err := pl.Do(req)
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		_ = resp.Body.Close()
	}

	run(auth.NewBearerTokenPolicyWithCache(cred, cache, "scope/.default"))
	run(auth.NewBearerTokenPolicyWithCache(cred, cache, "scope/.default"))

	if cred.calls != 1 {
		t.Fatalf("GetToken calls = %d, want 1 (shared cache reuses the token)", cred.calls)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./auth/ -run 'InMemoryTokenCache|SharedCache' -v`
Expected: FAIL — `auth.NewInMemoryTokenCache`/`NewBearerTokenPolicyWithCache` undefined.

- [ ] **Step 3: Create `auth/cache.go`**

```go
// auth/cache.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package auth

import "sync"

// TokenCache stores access tokens keyed by an opaque key (the SDK uses the
// space-joined scope set). Implementations must be safe for concurrent use.
type TokenCache interface {
	// Get returns the cached token for key and whether one was present.
	Get(key string) (AccessToken, bool)
	// Set stores token under key.
	Set(key string, token AccessToken)
}

// InMemoryTokenCache is a concurrency-safe in-memory [TokenCache].
type InMemoryTokenCache struct {
	mu     sync.Mutex
	tokens map[string]AccessToken
}

// NewInMemoryTokenCache returns an empty in-memory cache.
func NewInMemoryTokenCache() *InMemoryTokenCache {
	return &InMemoryTokenCache{tokens: make(map[string]AccessToken)}
}

// Get implements [TokenCache].
func (c *InMemoryTokenCache) Get(key string) (AccessToken, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok := c.tokens[key]
	return t, ok
}

// Set implements [TokenCache].
func (c *InMemoryTokenCache) Set(key string, token AccessToken) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tokens[key] = token
}
```

- [ ] **Step 4: Refactor `auth/bearer.go`**

Add `"strings"` to the stdlib import group. Replace the `BearerTokenPolicy` struct, its constructor, the `token` method, and the `fresh` method with:

```go
// BearerTokenPolicy attaches an "Authorization: Bearer <token>" header to every
// request, fetching the token from a [TokenCredential] and storing it in a
// [TokenCache]. The cached token is refreshed once it is within five minutes of
// expiry.
//
// The policy requires HTTPS and returns [ErrInsecureTransport] otherwise. It
// implements pipeline.Policy and is safe for concurrent use.
type BearerTokenPolicy struct {
	cred  TokenCredential
	scope []string
	key   string
	cache TokenCache

	mu sync.Mutex
}

// NewBearerTokenPolicy returns a policy that authenticates requests using cred
// for the given scopes, caching the token in a private in-memory cache.
func NewBearerTokenPolicy(cred TokenCredential, scopes ...string) *BearerTokenPolicy {
	return NewBearerTokenPolicyWithCache(cred, NewInMemoryTokenCache(), scopes...)
}

// NewBearerTokenPolicyWithCache is like [NewBearerTokenPolicy] but stores tokens
// in cache, which may be shared across policies so multiple clients reuse a
// cached token.
func NewBearerTokenPolicyWithCache(cred TokenCredential, cache TokenCache, scopes ...string) *BearerTokenPolicy {
	return &BearerTokenPolicy{
		cred:  cred,
		scope: scopes,
		key:   strings.Join(scopes, " "),
		cache: cache,
	}
}

// Do implements pipeline.Policy.
func (p *BearerTokenPolicy) Do(req *pipeline.Request) (*http.Response, error) {
	raw := req.Raw()
	if raw.URL == nil || raw.URL.Scheme != "https" {
		return nil, ErrInsecureTransport
	}
	token, err := p.token(req)
	if err != nil {
		return nil, err
	}
	raw.Header.Set(header.Authorization, "Bearer "+token)
	return req.Next()
}

// token returns a cached token when fresh, otherwise acquires a new one. The lock
// is held across the credential call so concurrent requests share a single
// refresh rather than stampeding the token endpoint.
func (p *BearerTokenPolicy) token(req *pipeline.Request) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if tok, ok := p.cache.Get(p.key); ok && fresh(tok) {
		return tok.Token, nil
	}
	tok, err := p.cred.GetToken(req.Raw().Context(), TokenRequestOptions{Scopes: p.scope})
	if err != nil {
		return "", fmt.Errorf("auth: acquire token: %w", err)
	}
	p.cache.Set(p.key, tok)
	return tok.Token, nil
}

// fresh reports whether tok is present and not near expiry. A zero ExpiresOn means
// the token never expires.
func fresh(tok AccessToken) bool {
	if tok.Token == "" {
		return false
	}
	if tok.ExpiresOn.IsZero() {
		return true
	}
	return time.Until(tok.ExpiresOn) > expiryWindow
}
```

(Keep the `expiryWindow` constant and the `ErrInsecureTransport` var as they are. The struct no longer has a `cached` field.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -race ./auth/ -v`
Expected: PASS — the new cache and shared-cache tests, AND the existing
`TestBearerAttachesHeaderAndCaches` (still one `GetToken` for three requests, now
via the private in-memory cache), `TestBearerRefusesInsecureScheme`,
`TestBearerPropagatesCredentialError`.

- [ ] **Step 6: Commit**

```bash
git add auth/cache.go auth/cache_test.go auth/bearer.go auth/bearer_test.go
git commit -m "feat(auth): add pluggable TokenCache; route BearerTokenPolicy through it"
```

---

## Task 2: umbrella `WithTokenCache`

**Files:**
- Modify: `options.go`, `client.go`
- Test: `client_test.go`

- [ ] **Step 1: Write the failing test**

Append to `client_test.go`. Add `"context"` and `"time"` to the stdlib import group and `"github.com/dexpace/go-sdk/auth"` to the dexpace group (if not already present from earlier auth tests).

```go
type countingCred struct{ calls int }

func (c *countingCred) GetToken(context.Context, auth.TokenRequestOptions) (auth.AccessToken, error) {
	c.calls++
	return auth.AccessToken{Token: "t", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

func TestWithTokenCacheSharedAcrossClients(t *testing.T) {
	t.Parallel()

	cred := &countingCred{}
	cache := auth.NewInMemoryTokenCache()

	for i := 0; i < 2; i++ {
		var captured *http.Request
		c := dexpace.New(
			dexpace.WithTransport(captureTransport(&captured)),
			dexpace.WithCredential(cred, "scope"),
			dexpace.WithTokenCache(cache),
		)
		req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
		resp, err := c.Do(req)
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		_ = resp.Body.Close()
	}

	if cred.calls != 1 {
		t.Fatalf("GetToken calls = %d, want 1 (token cache shared across clients)", cred.calls)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run TestWithTokenCache -v`
Expected: FAIL — `dexpace.WithTokenCache` undefined.

- [ ] **Step 3: Add the option in `options.go`**

Add a field to the `config` struct (near `credential`/`scopes`):
```go
	tokenCache auth.TokenCache
```
Add the option (after `WithCredential`):
```go
// WithTokenCache shares cache across the bearer-token policy installed by
// WithCredential, so multiple clients can reuse cached tokens. A nil cache or no
// credential means the default per-client in-memory cache is used.
func WithTokenCache(cache auth.TokenCache) Option {
	return func(c *config) { c.tokenCache = cache }
}
```

- [ ] **Step 4: Wire it in `client.go`**

In the auth `switch`, change the credential case to use the shared cache when set:
```go
	case cfg.credential != nil:
		cache := cfg.tokenCache
		if cache == nil {
			cache = auth.NewInMemoryTokenCache()
		}
		placements = append(placements,
			pipeline.At(pipeline.StageAuth, auth.NewBearerTokenPolicyWithCache(cfg.credential, cache, cfg.scopes...)))
```
(Leave the `basicAuth` and `apiKey` cases unchanged.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test . -v`
Expected: PASS — the new test plus all existing umbrella tests.

- [ ] **Step 6: Run the full suite**

Run: `go test ./...`
Expected: PASS across every package.

- [ ] **Step 7: Commit**

```bash
git add client.go options.go client_test.go
git commit -m "feat: add WithTokenCache to share bearer tokens across clients"
```

---

## Task 3: docs and full gate

**Files:**
- Modify: `doc.go`, `README.md`

- [ ] **Step 1: Mention the token cache in `doc.go`**

Read `doc.go`. Within the `package dexpace` doc comment (single contiguous `//`
block; no second package clause / no duplicate header), add:

```go
// WithTokenCache shares a bearer-token cache across clients (auth.TokenCache, with
// an in-memory default).
```

- [ ] **Step 2: Update `README.md`**

Read `README.md`. In the options/usage section, add a short entry: `WithTokenCache(cache)`
shares a bearer-token cache (an `auth.TokenCache`, in-memory by default) across
clients so a cached token is reused. Keep it tight; match the surrounding style.

- [ ] **Step 3: Run the full gate**

Run:
```bash
gofmt -l .
go vet ./...
go test -race ./...
```
Expected: `gofmt -l .` prints nothing; `go vet` clean; every package passes under
the race detector.

- [ ] **Step 4: Commit**

```bash
git add doc.go README.md
git commit -m "docs: document WithTokenCache"
```

---

## Self-Review notes (for the implementer)

- **Spec coverage:** `TokenCache` + `InMemoryTokenCache` + bearer refactor (Task 1);
  `WithTokenCache` umbrella (Task 2); docs (Task 3).
- **Behaviour preserved:** the existing `TestBearerAttachesHeaderAndCaches` passes
  unchanged (private in-memory cache via `NewBearerTokenPolicy`).
- **Type consistency:** `auth.TokenCache`, `auth.NewInMemoryTokenCache`,
  `auth.NewBearerTokenPolicyWithCache(cred, cache, scopes...)`, the package `fresh`
  helper, and `dexpace.WithTokenCache(cache)` used identically across tasks.
- **Concurrency:** `InMemoryTokenCache` is mutex-guarded (asserted under `-race`);
  the per-policy refresh lock is retained.
- **`make check`** green before opening the PR.
