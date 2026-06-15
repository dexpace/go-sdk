// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package auth

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/dexpace/go-sdk/header"
	"github.com/dexpace/go-sdk/pipeline"
)

// expiryWindow is how long before actual expiry a cached token is treated as
// stale, so a refresh happens before in-flight requests can start failing.
const expiryWindow = 5 * time.Minute

// ErrInsecureTransport is returned when a bearer token would be sent over a
// non-HTTPS connection. Sending credentials in clear text is refused to avoid
// leaking them.
var ErrInsecureTransport = errors.New("auth: refusing to send bearer token over an insecure (non-HTTPS) connection")

// BearerTokenPolicy attaches an "Authorization: Bearer <token>" header to every
// request, fetching and caching the token from a [TokenCredential]. The cached
// token is refreshed once it is within five minutes of expiry.
//
// The policy requires HTTPS and returns [ErrInsecureTransport] otherwise. It
// implements pipeline.Policy and is safe for concurrent use.
type BearerTokenPolicy struct {
	cred   TokenCredential
	scopes []string

	mu     sync.Mutex
	cached AccessToken
}

// NewBearerTokenPolicy returns a policy that authenticates requests using cred
// for the given scopes.
func NewBearerTokenPolicy(cred TokenCredential, scopes ...string) *BearerTokenPolicy {
	return &BearerTokenPolicy{cred: cred, scopes: scopes}
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

// token returns a cached token when fresh, otherwise acquires a new one. The
// lock is held across the credential call so concurrent requests share a single
// refresh rather than stampeding the token endpoint.
func (p *BearerTokenPolicy) token(req *pipeline.Request) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.fresh() {
		return p.cached.Token, nil
	}
	tok, err := p.cred.GetToken(req.Raw().Context(), TokenRequestOptions{Scopes: p.scopes})
	if err != nil {
		return "", fmt.Errorf("auth: acquire token: %w", err)
	}
	p.cached = tok
	return tok.Token, nil
}

// fresh reports whether the cached token is present and not near expiry. A zero
// ExpiresOn means the token never expires.
func (p *BearerTokenPolicy) fresh() bool {
	if p.cached.Token == "" {
		return false
	}
	if p.cached.ExpiresOn.IsZero() {
		return true
	}
	return time.Until(p.cached.ExpiresOn) > expiryWindow
}
