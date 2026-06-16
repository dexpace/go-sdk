// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dexpace/go-sdk/header"
	"github.com/dexpace/go-sdk/pipeline"
)

// expiryWindow is how long before actual expiry a cached token is treated as
// stale, so a refresh happens before in-flight requests can start failing.
const expiryWindow = 5 * time.Minute

// ErrInsecureTransport is returned when credentials would be sent over a
// non-HTTPS connection. Sending credentials in clear text is refused to avoid
// leaking them.
var ErrInsecureTransport = errors.New("auth: refusing to send credentials over an insecure (non-HTTPS) connection")

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
// cached token. Each policy serializes its own refresh; when the cached token is
// stale, two policies sharing a cache may refresh independently (last write wins).
// Cross-policy single-flight is not performed.
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
