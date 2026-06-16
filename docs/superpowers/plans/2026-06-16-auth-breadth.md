# Auth Breadth Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Basic auth and API-key credential policies (both HTTPS-only) to the `auth` package, with `dexpace.WithBasicAuth` / `dexpace.WithAPIKey` umbrella options.

**Architecture:** Two new pipeline policies in `auth` mirror `BearerTokenPolicy`: each refuses non-HTTPS (reusing the `ErrInsecureTransport` sentinel) and sets its credential header only when absent. The umbrella installs one auth policy at `StageAuth` with precedence bearer > basic > API key.

**Tech Stack:** Go 1.26+, standard library only (`encoding/base64` is not needed — `http.Request.SetBasicAuth` handles Basic). Zero third-party dependencies.

**Conventions every task must follow:**
- MIT license header on every `.go` file before the `package` clause:
  ```go
  // Copyright (c) 2026 dexpace and Omar Aljarrah.
  // Licensed under the MIT License. See LICENSE in the repository root for details.
  ```
- Import groups: stdlib, blank line, then `github.com/dexpace/go-sdk/...`.
- Tests use `t.Parallel()`; table-driven; local `transporterFunc` fakes; bodies closed via `t.Cleanup`. Tests MUST use `https://` URLs (the policies refuse non-HTTPS).
- Tools: Go 1.26.3; `gofumpt`/`golangci-lint` NOT installed — use `gofmt`, `go vet`, `go test -race`.
- Run commands from the repo root `/Users/omar/dexpace/go-sdk`.

---

## File Structure

| Path | Responsibility |
|---|---|
| `auth/basic.go` (new) + test | `BasicCredential`, `BasicAuthPolicy`, `NewBasicAuthPolicy` |
| `auth/apikey.go` (new) + test | `APIKeyPolicy`, `NewAPIKeyPolicy` |
| `auth/bearer.go` (modify) | generalize the `ErrInsecureTransport` message |
| `options.go` (modify) | `WithBasicAuth`, `WithAPIKey` + config fields |
| `client.go` (modify) | auth placement precedence |
| `client_test.go` (modify) | umbrella auth tests |
| `doc.go`, `README.md` (modify) | document |

---

## Task 1: Basic auth and API-key policies

**Files:**
- Create: `auth/basic.go`, `auth/apikey.go`
- Modify: `auth/bearer.go`
- Test: `auth/basic_test.go`, `auth/apikey_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// auth/basic_test.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package auth_test

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/dexpace/go-sdk/auth"
	"github.com/dexpace/go-sdk/pipeline"
)

func okTransport(seen *http.Request) transporterFunc {
	return func(req *http.Request) (*http.Response, error) {
		*seen = *req
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
	}
}

func TestBasicAuthAttachesHeader(t *testing.T) {
	t.Parallel()

	var seen http.Request
	pl := pipeline.New(okTransport(&seen),
		auth.NewBasicAuthPolicy(auth.BasicCredential{Username: "alice", Password: "s3cr3t"}))
	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	u, p, ok := seen.BasicAuth()
	if !ok || u != "alice" || p != "s3cr3t" {
		t.Fatalf("BasicAuth = (%q,%q,%v), want alice/s3cr3t/true", u, p, ok)
	}
}

func TestBasicAuthRefusesInsecure(t *testing.T) {
	t.Parallel()

	pl := pipeline.New(
		transporterFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("transport reached for insecure request")
			return nil, nil
		}),
		auth.NewBasicAuthPolicy(auth.BasicCredential{Username: "a", Password: "b"}),
	)
	req, _ := http.NewRequest(http.MethodGet, "http://api.example.test/", nil)
	if _, err := pl.Do(req); !errors.Is(err, auth.ErrInsecureTransport) {
		t.Fatalf("err = %v, want ErrInsecureTransport", err)
	}
}

func TestBasicAuthPreservesCallerHeader(t *testing.T) {
	t.Parallel()

	var seen http.Request
	pl := pipeline.New(okTransport(&seen),
		auth.NewBasicAuthPolicy(auth.BasicCredential{Username: "alice", Password: "s3cr3t"}))
	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
	req.Header.Set("Authorization", "Bearer caller-token")
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if got := seen.Header.Get("Authorization"); got != "Bearer caller-token" {
		t.Fatalf("Authorization = %q, want the caller value preserved", got)
	}
}
```

```go
// auth/apikey_test.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package auth_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/dexpace/go-sdk/auth"
	"github.com/dexpace/go-sdk/pipeline"
)

func TestAPIKeyAttachesHeader(t *testing.T) {
	t.Parallel()

	var seen http.Request
	pl := pipeline.New(okTransport(&seen), auth.NewAPIKeyPolicy("X-API-Key", "secret-key"))
	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if got := seen.Header.Get("X-API-Key"); got != "secret-key" {
		t.Fatalf("X-API-Key = %q, want secret-key", got)
	}
}

func TestAPIKeyRefusesInsecure(t *testing.T) {
	t.Parallel()

	pl := pipeline.New(
		transporterFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("transport reached for insecure request")
			return nil, nil
		}),
		auth.NewAPIKeyPolicy("X-API-Key", "secret-key"),
	)
	req, _ := http.NewRequest(http.MethodGet, "http://api.example.test/", nil)
	if _, err := pl.Do(req); !errors.Is(err, auth.ErrInsecureTransport) {
		t.Fatalf("err = %v, want ErrInsecureTransport", err)
	}
}

func TestAPIKeyPreservesCallerHeader(t *testing.T) {
	t.Parallel()

	var seen http.Request
	pl := pipeline.New(okTransport(&seen), auth.NewAPIKeyPolicy("X-API-Key", "secret-key"))
	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
	req.Header.Set("X-API-Key", "caller-key")
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if got := seen.Header.Get("X-API-Key"); got != "caller-key" {
		t.Fatalf("X-API-Key = %q, want the caller value preserved", got)
	}
}

func TestNewAPIKeyPolicyPanicsOnEmptyHeader(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for empty header name")
		}
	}()
	_ = auth.NewAPIKeyPolicy("", "key")
}
```

NOTE: `transporterFunc` is already defined in `auth/bearer_test.go` (package `auth_test`) — reuse it; do NOT redefine it. `okTransport` is new (defined in basic_test.go above) and shared with apikey_test.go (same package).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./auth/ -run 'BasicAuth|APIKey' -v`
Expected: FAIL — `NewBasicAuthPolicy`/`NewAPIKeyPolicy`/`BasicCredential` undefined.

- [ ] **Step 3: Generalize the sentinel message in `auth/bearer.go`**

Replace the `ErrInsecureTransport` declaration's message:
```go
// ErrInsecureTransport is returned when credentials would be sent over a
// non-HTTPS connection. Sending credentials in clear text is refused to avoid
// leaking them.
var ErrInsecureTransport = errors.New("auth: refusing to send credentials over an insecure (non-HTTPS) connection")
```
(Only the doc comment and the message string change; the variable identity is unchanged.)

- [ ] **Step 4: Create `auth/basic.go`**

```go
// auth/basic.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package auth

import (
	"net/http"

	"github.com/dexpace/go-sdk/header"
	"github.com/dexpace/go-sdk/pipeline"
)

// BasicCredential holds HTTP Basic auth credentials.
type BasicCredential struct {
	Username string
	Password string
}

// BasicAuthPolicy sets an "Authorization: Basic ..." header on each request that
// does not already carry one. It requires HTTPS and returns [ErrInsecureTransport]
// otherwise. It implements pipeline.Policy and is safe for concurrent use.
type BasicAuthPolicy struct {
	cred BasicCredential
}

// NewBasicAuthPolicy returns a policy that authenticates requests with cred.
func NewBasicAuthPolicy(cred BasicCredential) *BasicAuthPolicy {
	return &BasicAuthPolicy{cred: cred}
}

// Do implements pipeline.Policy.
func (p *BasicAuthPolicy) Do(req *pipeline.Request) (*http.Response, error) {
	raw := req.Raw()
	if raw.URL == nil || raw.URL.Scheme != "https" {
		return nil, ErrInsecureTransport
	}
	if raw.Header.Get(header.Authorization) == "" {
		raw.SetBasicAuth(p.cred.Username, p.cred.Password)
	}
	return req.Next()
}
```

- [ ] **Step 5: Create `auth/apikey.go`**

```go
// auth/apikey.go
// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package auth

import (
	"net/http"

	"github.com/dexpace/go-sdk/pipeline"
)

// APIKeyPolicy sets a fixed header to an API key on each request that does not
// already carry that header. It requires HTTPS and returns [ErrInsecureTransport]
// otherwise. It implements pipeline.Policy and is safe for concurrent use.
type APIKeyPolicy struct {
	header string
	key    string
}

// NewAPIKeyPolicy returns a policy that sets header to key on each request. It
// panics if header is empty, which is a programming error.
func NewAPIKeyPolicy(header, key string) *APIKeyPolicy {
	if header == "" {
		panic("auth: NewAPIKeyPolicy requires a non-empty header name")
	}
	return &APIKeyPolicy{header: header, key: key}
}

// Do implements pipeline.Policy.
func (p *APIKeyPolicy) Do(req *pipeline.Request) (*http.Response, error) {
	raw := req.Raw()
	if raw.URL == nil || raw.URL.Scheme != "https" {
		return nil, ErrInsecureTransport
	}
	if raw.Header.Get(p.header) == "" {
		raw.Header.Set(p.header, p.key)
	}
	return req.Next()
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./auth/ -v`
Expected: PASS — the new Basic and API-key tests plus all existing bearer tests (the sentinel identity is unchanged, so `TestBearerRefusesInsecureScheme` still passes).

- [ ] **Step 7: Commit**

```bash
git add auth/
git commit -m "feat(auth): add Basic auth and API-key credential policies"
```

---

## Task 2: umbrella `WithBasicAuth` / `WithAPIKey`

**Files:**
- Modify: `options.go`, `client.go`
- Test: `client_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `client_test.go` (it has `captureTransport`, `transporterFunc`; imports include net/http, testing, dexpace). 

```go
func TestWithBasicAuth(t *testing.T) {
	t.Parallel()

	var captured *http.Request
	c := dexpace.New(
		dexpace.WithTransport(captureTransport(&captured)),
		dexpace.WithBasicAuth("alice", "s3cr3t"),
	)
	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	u, p, ok := captured.BasicAuth()
	if !ok || u != "alice" || p != "s3cr3t" {
		t.Fatalf("BasicAuth = (%q,%q,%v), want alice/s3cr3t/true", u, p, ok)
	}
}

func TestWithAPIKey(t *testing.T) {
	t.Parallel()

	var captured *http.Request
	c := dexpace.New(
		dexpace.WithTransport(captureTransport(&captured)),
		dexpace.WithAPIKey("X-API-Key", "secret-key"),
	)
	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if got := captured.Header.Get("X-API-Key"); got != "secret-key" {
		t.Fatalf("X-API-Key = %q, want secret-key", got)
	}
}
```

NOTE: `captureTransport` must produce a request whose URL scheme is `https` so the auth policies don't reject it — the test uses an `https://` request URL, which `captureTransport` forwards. Confirm `captureTransport` does not rewrite the URL.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test . -run 'TestWithBasicAuth|TestWithAPIKey' -v`
Expected: FAIL — `dexpace.WithBasicAuth`/`WithAPIKey` undefined.

- [ ] **Step 3: Add fields and options in `options.go`**

Add fields to the `config` struct (after `scopes []string`):
```go
	basicAuth *auth.BasicCredential
	apiKey    apiKeyConfig
```
Add an unexported helper type near the `config` struct:
```go
type apiKeyConfig struct {
	header string
	key    string
	set    bool
}
```
Add the options (after `WithCredential`):
```go
// WithBasicAuth authenticates requests with HTTP Basic auth. Like all credential
// policies it requires HTTPS. If multiple auth options are set, the precedence is
// WithCredential, then WithBasicAuth, then WithAPIKey.
func WithBasicAuth(username, password string) Option {
	return func(c *config) {
		c.basicAuth = &auth.BasicCredential{Username: username, Password: password}
	}
}

// WithAPIKey authenticates requests by setting header to key (HTTPS-only). See
// WithBasicAuth for the precedence when multiple auth options are set.
func WithAPIKey(header, key string) Option {
	return func(c *config) {
		c.apiKey = apiKeyConfig{header: header, key: key, set: true}
	}
}
```
(`auth` is already imported in `options.go`.)

- [ ] **Step 4: Update the auth placement in `client.go`**

Replace the current auth block:
```go
	if cfg.credential != nil {
		placements = append(placements,
			pipeline.At(pipeline.StageAuth, auth.NewBearerTokenPolicy(cfg.credential, cfg.scopes...)))
	}
```
with a precedence chain (a client should configure at most one auth method):
```go
	switch {
	case cfg.credential != nil:
		placements = append(placements,
			pipeline.At(pipeline.StageAuth, auth.NewBearerTokenPolicy(cfg.credential, cfg.scopes...)))
	case cfg.basicAuth != nil:
		placements = append(placements,
			pipeline.At(pipeline.StageAuth, auth.NewBasicAuthPolicy(*cfg.basicAuth)))
	case cfg.apiKey.set:
		placements = append(placements,
			pipeline.At(pipeline.StageAuth, auth.NewAPIKeyPolicy(cfg.apiKey.header, cfg.apiKey.key)))
	}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test . -v`
Expected: PASS — the two new tests plus all existing umbrella tests.

- [ ] **Step 6: Run the full suite**

Run: `go test ./...`
Expected: PASS across every package.

- [ ] **Step 7: Commit**

```bash
git add client.go options.go client_test.go
git commit -m "feat: add WithBasicAuth and WithAPIKey umbrella options"
```

---

## Task 3: docs and full gate

**Files:**
- Modify: `doc.go`, `README.md`

- [ ] **Step 1: Mention the new auth methods in `doc.go`**

Read `doc.go`. Within the `package dexpace` doc comment (single contiguous `//` block above `package dexpace`; no second package clause / no duplicate header), add:

```go
// Beyond bearer tokens (WithCredential), WithBasicAuth and WithAPIKey authenticate
// requests with HTTP Basic auth or an API-key header; both require HTTPS.
```

- [ ] **Step 2: Update `README.md`**

Read `README.md`. In the options/usage section, add short entries for `WithBasicAuth(user, pass)` and `WithAPIKey(header, key)` — both HTTPS-only credential methods alongside `WithCredential`. Match the surrounding style; keep it tight.

- [ ] **Step 3: Run the full gate**

Run:
```bash
gofmt -l .
go vet ./...
go test -race ./...
```
Expected: `gofmt -l .` prints nothing; `go vet` clean; every package passes under the race detector.

- [ ] **Step 4: Commit**

```bash
git add doc.go README.md
git commit -m "docs: document WithBasicAuth and WithAPIKey"
```

---

## Self-Review notes (for the implementer)

- **Spec coverage:** Basic auth policy + API-key policy + sentinel message generalization (Task 1); umbrella options + precedence (Task 2); docs (Task 3). Deferred items (challenge handlers, token cache) are intentionally not implemented.
- **Type consistency:** `auth.BasicCredential`, `auth.NewBasicAuthPolicy`, `auth.NewAPIKeyPolicy(header, key)`, `dexpace.WithBasicAuth(user, pass)`, `dexpace.WithAPIKey(header, key)`, and the `apiKeyConfig{header,key,set}` helper are used identically across tasks.
- **HTTPS-only:** all three policies refuse non-HTTPS via `ErrInsecureTransport`; tests use `https://` URLs.
- **Sentinel identity unchanged:** only the message string/doc changes, so `errors.Is(err, auth.ErrInsecureTransport)` continues to work, including the existing bearer test.
- **`make check`** green before opening the PR.
