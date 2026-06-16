# Auth breadth — design

**Date:** 2026-06-16
**Status:** Approved (standing delegation); ready for implementation planning
**Subsystem:** #6 of the Go SDK platform-parity roadmap

## Context

The `auth` package today supports only bearer tokens (`TokenCredential`,
`StaticToken`, `BearerTokenPolicy`, HTTPS-only via `ErrInsecureTransport`). The
Java/Python SDKs also offer Basic auth, key credentials, WWW-Authenticate
challenge handlers (Basic/Digest), and a pluggable token cache.

## Decisions

1. **Scope: Basic auth + API-key credential**, with umbrella options. These are
   the common, low-risk authentication methods modern APIs use beyond bearer.
2. **Both are HTTPS-only**, reusing the existing `ErrInsecureTransport` sentinel
   (whose message is generalized from "bearer token" to "credentials").
3. **Set only when absent** — a caller-supplied `Authorization`/key header wins,
   matching the other header policies.
4. **Deferred (documented):** WWW-Authenticate challenge handlers (Basic/Digest).
   RFC 7616 Digest is complex and security-sensitive, and modern REST APIs rarely
   use challenge-driven auth — YAGNI until a concrete need arises. Also deferred:
   a pluggable token cache (additive parity that would refactor the working
   `BearerTokenPolicy`; its internal cache suffices for now).

## Architecture

### Basic auth (`auth/basic.go`)

```go
// BasicCredential holds HTTP Basic auth credentials.
type BasicCredential struct {
	Username string
	Password string
}

// BasicAuthPolicy sets "Authorization: Basic base64(user:pass)" on each request
// that lacks an Authorization header. It requires HTTPS and returns
// ErrInsecureTransport otherwise. Safe for concurrent use.
type BasicAuthPolicy struct { /* cred */ }

func NewBasicAuthPolicy(cred BasicCredential) *BasicAuthPolicy
func (p *BasicAuthPolicy) Do(req *pipeline.Request) (*http.Response, error)
```

`Do`: refuse non-HTTPS (`raw.URL == nil || raw.URL.Scheme != "https"` →
`ErrInsecureTransport`); if `Authorization` is absent, `raw.SetBasicAuth(user, pass)`;
then `req.Next()`.

### API-key credential (`auth/apikey.go`)

```go
// APIKeyPolicy stamps a fixed header with an API key on each request that lacks
// that header. It requires HTTPS and returns ErrInsecureTransport otherwise.
// Safe for concurrent use.
type APIKeyPolicy struct { /* header, key */ }

// NewAPIKeyPolicy returns a policy that sets header to key. It panics if header
// is empty (a programmer error at construction).
func NewAPIKeyPolicy(header, key string) *APIKeyPolicy
func (p *APIKeyPolicy) Do(req *pipeline.Request) (*http.Response, error)
```

`Do`: refuse non-HTTPS; if `header` is absent on the request, set it to `key`;
then `req.Next()`. (An API key is a credential, so it is HTTPS-only to avoid
leaking it in clear text.)

### Sentinel message generalization (`auth/bearer.go`)

`ErrInsecureTransport`'s message changes from
`"...refusing to send bearer token over..."` to
`"auth: refusing to send credentials over an insecure (non-HTTPS) connection"`,
since it now also guards Basic and API-key policies. The sentinel identity is
unchanged, so existing `errors.Is` checks still pass.

### Umbrella options (`options.go` / `client.go`)

```go
// WithBasicAuth authenticates requests with HTTP Basic auth (HTTPS-only).
func WithBasicAuth(username, password string) Option

// WithAPIKey authenticates requests by setting header to key (HTTPS-only).
func WithAPIKey(header, key string) Option
```

`config` gains `basicAuth *auth.BasicCredential` and `apiKey struct{ header, key string; set bool }`
(or equivalent). In `New`, the auth placement chooses one policy with documented
precedence (a client should configure at most one auth method):

1. `cfg.credential != nil` → bearer (unchanged),
2. else `cfg.basicAuth != nil` → basic,
3. else `cfg.apiKey.set` → API key.

The chosen policy is placed `At(pipeline.StageAuth)`. Precedence is
order-independent.

## Edge cases

- Non-HTTPS request → `ErrInsecureTransport` (before any header is set or the
  request is sent), for all three auth policies.
- A caller-supplied `Authorization` (basic) or key header is preserved (set only
  when absent).
- `NewAPIKeyPolicy("", key)` panics (programmer error), consistent with
  `pipeline.New(nil)`.
- `raw.URL == nil` → treated as insecure (`ErrInsecureTransport`).
- Empty username/password is allowed (Basic with empty fields is valid per spec);
  not validated.

## Package layout

| Path | Change |
|---|---|
| `auth/basic.go` (+ test) | `BasicCredential`, `BasicAuthPolicy`, `NewBasicAuthPolicy` |
| `auth/apikey.go` (+ test) | `APIKeyPolicy`, `NewAPIKeyPolicy` |
| `auth/bearer.go` | generalize the `ErrInsecureTransport` message |
| `options.go` (+ test) | `WithBasicAuth`, `WithAPIKey` + fields |
| `client.go` | auth placement precedence |
| `doc.go`, `README.md` | document |

## Testing

- Basic: attaches `Authorization: Basic base64(user:pass)` over HTTPS (decode and
  verify); refuses non-HTTPS with `ErrInsecureTransport`; preserves a
  caller-supplied `Authorization`.
- API key: sets the configured header to the key over HTTPS; refuses non-HTTPS;
  preserves a caller-supplied header value; `NewAPIKeyPolicy("", ...)` panics.
- Umbrella: `WithBasicAuth` / `WithAPIKey` install the policy (verify the header
  reaches a capturing transport over an `https://` URL); precedence (bearer beats
  basic beats API key) when multiple are set.
- Existing bearer tests still pass (sentinel identity unchanged).
- Table-driven, parallel; stdlib-only; `gofmt`/`go vet`/`go test -race` clean.

## Out of scope (deferred)

- WWW-Authenticate challenge handlers (Basic, Digest/RFC 7616).
- Pluggable token cache (`TokenCache` interface + in-memory default).
- OAuth client-credentials/device-code credential helpers (users implement
  `TokenCredential`).
