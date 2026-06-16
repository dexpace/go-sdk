# Pluggable token cache — design

**Date:** 2026-06-16
**Status:** Approved (standing delegation); ready for implementation planning
**Subsystem:** deferred-feature #4 (pluggable token cache from the auth-breadth roadmap item)

## Context

`BearerTokenPolicy` caches one token in a private field, refreshed within five
minutes of expiry. That cache is per-policy, so two policies (e.g. two clients to
the same API) each fetch their own token. A pluggable `TokenCache` lets callers
share (or persist) tokens. Java/Python expose the same seam with an in-memory
default.

## Decisions

1. **`TokenCache` interface + in-memory default.** Keyed by the scope set, with a
   thread-safe `InMemoryTokenCache`.
2. **Preserve current behaviour.** `NewBearerTokenPolicy(cred, scopes...)` is
   unchanged and uses a fresh per-policy `InMemoryTokenCache` — identical to today
   (existing tests pass without edits).
3. **Add, don't break, the constructor.** A second constructor
   `NewBearerTokenPolicyWithCache(cred, cache, scopes...)` injects a shared cache;
   the variadic `scopes` parameter rules out a functional-options form on the
   existing constructor.
4. **Per-policy refresh lock retained.** The existing mutex still serializes a
   single policy's refresh (no stampede). A shared cache populated by one policy is
   reused by others via the freshness check; cross-policy refresh coordination is
   out of scope (acceptable: at worst a duplicate fetch, last write wins).
5. **Umbrella `WithTokenCache(cache)`.** Wires a shared cache into the bearer
   policy `WithCredential` installs.

## Architecture

### `auth.TokenCache` (`auth/cache.go`)

```go
// TokenCache stores access tokens keyed by an opaque key (the SDK uses the
// space-joined scope set). Implementations must be safe for concurrent use.
type TokenCache interface {
	Get(key string) (AccessToken, bool)
	Set(key string, token AccessToken)
}

// InMemoryTokenCache is a concurrency-safe in-memory TokenCache.
type InMemoryTokenCache struct { /* mu, map */ }

// NewInMemoryTokenCache returns an empty in-memory cache.
func NewInMemoryTokenCache() *InMemoryTokenCache

func (c *InMemoryTokenCache) Get(key string) (AccessToken, bool)
func (c *InMemoryTokenCache) Set(key string, token AccessToken)
```

### `BearerTokenPolicy` refactor (`auth/bearer.go`)

- Replace the `cached AccessToken` field with `cache TokenCache` and a precomputed
  `key string` (= `strings.Join(scopes, " ")`).
- `NewBearerTokenPolicy(cred, scopes...)` delegates to
  `NewBearerTokenPolicyWithCache(cred, NewInMemoryTokenCache(), scopes...)`.
- `token`: under the lock, `cache.Get(key)`; if present and fresh, return it; else
  `cred.GetToken`, `cache.Set(key, tok)`, return.
- Freshness becomes a package helper `fresh(tok AccessToken) bool` (empty → false;
  zero `ExpiresOn` → never expires → true; else `time.Until(ExpiresOn) >
  expiryWindow`). The `expiryWindow` constant is unchanged.

```go
// NewBearerTokenPolicyWithCache returns a bearer policy that stores tokens in
// cache (shareable across policies). NewBearerTokenPolicy uses a private
// in-memory cache.
func NewBearerTokenPolicyWithCache(cred TokenCredential, cache TokenCache, scopes ...string) *BearerTokenPolicy
```

### Umbrella `WithTokenCache` (`options.go` / `client.go`)

```go
// WithTokenCache shares cache across the bearer-token policy installed by
// WithCredential, so multiple clients can reuse cached tokens. Ignored unless a
// credential is configured.
func WithTokenCache(cache auth.TokenCache) Option
```

`config` gains `tokenCache auth.TokenCache`. In `New`, the bearer case builds the
policy with `NewBearerTokenPolicyWithCache(cred, cacheOrDefault, scopes...)` where
`cacheOrDefault` is `cfg.tokenCache` when set, else `auth.NewInMemoryTokenCache()`.

## Edge cases

- A zero-`ExpiresOn` token is cached indefinitely (never refreshed) — unchanged.
- Two policies sharing a cache: the second sees the first's fresh token and skips
  the fetch; if both miss concurrently, both fetch and the last `Set` wins (benign).
- Distinct scope sets cache under distinct keys, so a shared cache serves multiple
  scopes correctly.
- `WithTokenCache(nil)` → treated as unset (default in-memory cache used).
- The `Do` HTTPS guard, header set, and error wrapping are unchanged.

## Package layout

| Path | Change |
|---|---|
| `auth/cache.go` (new) + test | `TokenCache`, `InMemoryTokenCache` |
| `auth/bearer.go` (modify) | use `TokenCache`; add `NewBearerTokenPolicyWithCache`; `fresh` helper |
| `auth/bearer_test.go` (modify) | add a shared-cache test (existing tests unchanged) |
| `options.go`, `client.go` (modify) | `WithTokenCache` + wiring |
| `client_test.go` (modify) | `WithTokenCache` reuse test |
| `doc.go`, `README.md` | document |

## Testing

- `InMemoryTokenCache`: Get on a missing key → (zero, false); Set then Get →
  (token, true); concurrent Get/Set under `-race` (a short goroutine loop).
- Bearer behaviour preserved: the existing `TestBearerAttachesHeaderAndCaches`
  still passes (per-policy cache, one `GetToken` for three requests).
- Shared cache: two `BearerTokenPolicy` instances built with the same cache and a
  counting credential — only the first triggers `GetToken`; the second reuses the
  cached token (counter stays 1).
- Umbrella: two clients built with `WithCredential(countingCred)` + a shared
  `WithTokenCache(cache)` issue one request each; `GetToken` is called once.
- Table-driven where natural, parallel; stdlib-only; `gofmt`/`go vet`/`go test
  -race` clean.

## Out of scope (deferred)

- Cross-policy single-flight refresh (per-key locking in the cache). The per-policy
  lock plus freshness check is sufficient; add single-flight if a real stampede
  appears.
- A persistent (disk/redis) cache implementation — users implement `TokenCache`.
- Negative caching of token-fetch errors.
