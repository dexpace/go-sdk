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
