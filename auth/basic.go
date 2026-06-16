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
