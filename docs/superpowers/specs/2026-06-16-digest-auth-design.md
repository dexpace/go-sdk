# HTTP Digest authentication — design

**Date:** 2026-06-16
**Status:** Approved (standing delegation); ready for implementation
**Subsystem:** deferred-feature #5 (WWW-Authenticate Digest, from the auth-breadth roadmap item)

## Context

The SDK has Bearer, Basic, and API-key credential policies. Digest Access
Authentication (RFC 7616) completes auth parity with the Java/Python ports. Digest
is challenge-driven: the server answers an unauthenticated request with `401` and
a `WWW-Authenticate: Digest …` challenge (realm, nonce, qop, algorithm, opaque);
the client hashes its credentials with the challenge and retries with an
`Authorization: Digest …` header. Unlike Basic, the password is never sent — only
a keyed hash.

## Decisions

1. **Reactive, then preemptive.** The policy sends the request; on a `401` Digest
   challenge it computes the response and retries **once**. It caches the challenge
   and applies Digest preemptively (with an incrementing nonce count `nc`) on
   subsequent requests, re-challenging only when the server issues a new nonce
   (stale). At most two sends per `Do`, so the loop is bounded.
2. **Algorithms: MD5 and SHA-256, plus `-sess` variants, `qop=auth` or none.**
   These cover essentially all real Digest servers. `SHA-512-256`, `qop=auth-int`
   (requires hashing the body), and `userhash` are out of scope (documented).
3. **HTTPS-only, like every other credential policy.** Returns
   `auth.ErrInsecureTransport` for a non-`https` URL. Digest was designed for
   cleartext HTTP, but this SDK refuses credentials over insecure transports
   uniformly; the username and a replayable response hash still travel in the
   header, so the guard is kept for consistency and defense in depth.
4. **`BasicCredential` reused** for username/password (no new credential type).
5. **Deterministic test seam.** An unexported `newCnonce func() (string, error)`
   field (default `crypto/rand`) lets package-internal tests inject a fixed cnonce
   and assert against the RFC 7616 §3.9.1 published response vectors.
6. **Umbrella `WithDigestAuth(username, password)`** at `StageAuth`. Precedence
   when several auth options are set: `WithCredential` → `WithBasicAuth` →
   `WithAPIKey` → `WithDigestAuth`.

## Architecture

### `auth/digest.go`

```go
type DigestAuthPolicy struct {
    cred      BasicCredential
    newCnonce func() (string, error) // test seam; default randomCnonce

    mu        sync.Mutex
    challenge *digestChallenge // last seen; nil until first 401
    nc        uint64           // nonce count for the current challenge
}

func NewDigestAuthPolicy(cred BasicCredential) *DigestAuthPolicy
func (p *DigestAuthPolicy) Do(req *pipeline.Request) (*http.Response, error)
```

**`Do` flow:**
1. HTTPS guard → `ErrInsecureTransport`.
2. Preemptive: if a challenge is cached and the request has no `Authorization`,
   attach `Digest …` with the next `nc`.
3. `resp, err := req.Next()`. Return early on error or non-401.
4. Parse the `WWW-Authenticate` values; if no Digest challenge, return the 401.
5. `RewindBody()`; if it fails (non-replayable body), return the 401 (can't retry).
6. Adopt the challenge (reset `nc` if the nonce changed), compute `Authorization`,
   drain+close the 401 body, set the header, and `return req.Next()` (the one retry).

**Challenge + crypto helpers (all unexported):**
- `digestChallenge{realm, nonce, opaque, algorithm, qopAuth bool, sess bool, hashFactory func() hash.Hash}`.
- `parseChallenge([]string) *digestChallenge` — picks the `Digest` scheme from the
  `WWW-Authenticate` header value(s); requires `realm`+`nonce`; selects the hash via
  `algorithm` (default MD5); records whether `qop` offers `auth`. One scheme per
  header value (multi-scheme single-line not supported — documented).
- `parseAuthParams(string) map[string]string` — quote-aware `key=value` scanner
  (handles commas inside quoted values and `qop="auth,auth-int"`).
- `authorization(ch, nc, method, uri)` — RFC 7616 computation:
  `HA1 = H(user:realm:pass)` (sess: `H(HA1:nonce:cnonce)`),
  `HA2 = H(method:uri)`,
  `response = H(HA1:nonce:nc:cnonce:auth:HA2)` for `qop=auth` else `H(HA1:nonce:HA2)`.
  `uri` is `raw.URL.RequestURI()`. Emits quoted `username/realm/nonce/uri/cnonce/
  response/opaque` and token `algorithm/qop=auth/nc`.
- `randomCnonce()` — 16 bytes from `crypto/rand`, hex-encoded.
- `drainClose(resp)` — discard ≤4 KiB then close, so the keep-alive connection is
  reusable for the retry.

### Umbrella wiring

`options.go`: `config.digestAuth *auth.BasicCredential`; `WithDigestAuth`.
`client.go`: a `case cfg.digestAuth != nil` in the auth switch installing
`auth.NewDigestAuthPolicy(*cfg.digestAuth)` at `StageAuth`.

## Edge cases

- Non-replayable body + 401 → the 401 is returned (no retry); documented.
- Server omits `algorithm` → MD5, and the response omits the `algorithm` param.
- Stale nonce (a preemptive attempt gets a fresh 401) → adopt the new challenge,
  reset `nc`, retry once.
- Concurrent first challenges may both use `nc=00000001` with distinct cnonces;
  acceptable (cnonce disambiguates; strict servers re-challenge). Documented.
- `crypto/rand` failure → surfaced as an error from `Do`.

## Package layout

| Path | Change |
|---|---|
| `auth/digest.go` (new) | `DigestAuthPolicy` + challenge/crypto helpers |
| `auth/digest_test.go` (new, `auth`) | RFC vectors via injected cnonce; parser tests |
| `auth/digest_roundtrip_test.go` (new, `auth_test`) | httptest 401→retry→200 round trip |
| `options.go`, `client.go` (modify) | `WithDigestAuth` + wiring |
| `client_test.go` (modify) | end-to-end `WithDigestAuth` over a stub/httptest |
| `doc.go`, `README.md`, `CLAUDE.md` | document |

## Testing

- **RFC 7616 §3.9.1 vectors** (deterministic via injected cnonce): username
  `Mufasa`, password `Circle of Life`, realm `http-auth@example.org`, the RFC nonce
  and cnonce, `uri=/dir/index.html`, `GET`, `qop=auth` — assert SHA-256 response
  `753927fa0e85d155564e2e272a28d1802ca10daf4496794697cf8db5856cb6c1` and MD5
  response `8ca523f5e9506fed4657c9700eebdbec`.
- **Parser**: realm/nonce/opaque/qop/algorithm extracted; quoted commas and
  `qop="auth,auth-int"` handled; a non-Digest scheme ignored.
- **Round trip**: an httptest server issues a 401 challenge, then validates the
  retried `Authorization` by recomputing the response from the known password;
  asserts a single challenge then `200`, and `nc` increments on a second request.
- **Insecure transport**: an `http://` URL returns `ErrInsecureTransport`.
- Table-driven, parallel; stdlib-only; `gofmt`/`go vet`/`go test -race` clean.

## Out of scope (deferred)

- `qop=auth-int`, `SHA-512-256`, `userhash=true`, and multi-scheme single-header
  parsing. Add if a real server requires them.
