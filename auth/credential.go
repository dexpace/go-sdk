// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package auth

import (
	"context"
	"time"
)

// AccessToken is a bearer token together with the instant at which it expires.
type AccessToken struct {
	// Token is the opaque bearer token value.
	Token string
	// ExpiresOn is when the token stops being valid. A zero value means the token
	// never expires (it will be cached indefinitely).
	ExpiresOn time.Time
}

// TokenRequestOptions carries the parameters of a token request.
type TokenRequestOptions struct {
	// Scopes are the resource scopes the token must be valid for.
	Scopes []string
}

// TokenCredential supplies bearer tokens on demand. Implementations must be safe
// for concurrent use, since a single credential is shared across every request
// that flows through a pipeline.
type TokenCredential interface {
	GetToken(ctx context.Context, opts TokenRequestOptions) (AccessToken, error)
}

// StaticToken is a [TokenCredential] that always returns the same token. It is
// handy for tests and for APIs authenticated with a long-lived key.
type StaticToken string

// GetToken returns the static token. It implements [TokenCredential].
func (s StaticToken) GetToken(context.Context, TokenRequestOptions) (AccessToken, error) {
	return AccessToken{Token: string(s)}, nil
}
