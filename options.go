// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package dexpace

import (
	"log/slog"

	"github.com/dexpace/go-sdk/auth"
	"github.com/dexpace/go-sdk/idempotency"
	"github.com/dexpace/go-sdk/pipeline"
	"github.com/dexpace/go-sdk/retry"
)

// Option configures a [Client] built by [New]. Options are applied in the order
// given; later options override earlier ones for the same setting.
type Option func(*config)

type config struct {
	transport  pipeline.Transporter
	retry      *retry.Options
	credential auth.TokenCredential
	scopes     []string
	logger     *slog.Logger
	logging    bool
	date       bool
	userAgent  string
	custom     []pipeline.Policy

	noIdempotency bool
	idempotency   *idempotency.Options
	before        []pipeline.Placement
	after         []pipeline.Placement
}

// WithTransport sets the terminal transport. When unset, the default net/http
// transport ([github.com/dexpace/go-sdk/transport.New]) is used. Useful for
// tests (a stub transporter) or to share a tuned *http.Client.
func WithTransport(t pipeline.Transporter) Option {
	return func(c *config) { c.transport = t }
}

// WithRetry configures the retry policy. When unset, a default retry policy is
// still installed; pass retry.Options{MaxRetries: -1} to disable retries.
func WithRetry(opts retry.Options) Option {
	return func(c *config) { c.retry = &opts }
}

// WithCredential installs a bearer-token auth policy using cred and the given
// scopes. Without it, no Authorization header is added.
func WithCredential(cred auth.TokenCredential, scopes ...string) Option {
	return func(c *config) {
		c.credential = cred
		c.scopes = scopes
	}
}

// WithLogging enables structured request/response logging via log/slog. A nil
// logger uses slog.Default().
func WithLogging(logger *slog.Logger) Option {
	return func(c *config) {
		c.logging = true
		c.logger = logger
	}
}

// WithoutIdempotency disables the default idempotency-key policy.
func WithoutIdempotency() Option {
	return func(c *config) { c.noIdempotency = true }
}

// WithIdempotency configures the idempotency-key policy (which is on by
// default). Passing custom options also re-enables it if a prior
// WithoutIdempotency was set. Pass the zero idempotency.Options for the default
// behaviour (POST only, the Idempotency-Key header, and UUIDv4 keys).
func WithIdempotency(opts idempotency.Options) Option {
	return func(c *config) {
		c.idempotency = &opts
		c.noIdempotency = false
	}
}

// WithPolicyBefore inserts a custom policy immediately before (outside) the given
// stage, so it wraps that stage and everything inner to it. The stage need not
// be occupied by a built-in policy. Multiple insertions at the same position run
// in the order added.
func WithPolicyBefore(stage pipeline.Stage, p pipeline.Policy) Option {
	return func(c *config) { c.before = append(c.before, pipeline.Before(stage, p)) }
}

// WithPolicyAfter inserts a custom policy immediately after (inside) the given
// stage, so the named stage wraps it. The stage need not be occupied by a
// built-in policy. Multiple insertions at the same position run in the order
// added.
func WithPolicyAfter(stage pipeline.Stage, p pipeline.Policy) Option {
	return func(c *config) { c.after = append(c.after, pipeline.After(stage, p)) }
}

// WithDate stamps a Date header (RFC 1123) on each request that lacks one. Off
// by default; net/http does not set a request Date and most REST APIs do not
// need it, but some request-signing schemes require it.
func WithDate() Option {
	return func(c *config) { c.date = true }
}

// WithUserAgent overrides the default User-Agent ("dexpace-go-sdk/<version>").
func WithUserAgent(ua string) Option {
	return func(c *config) { c.userAgent = ua }
}

// WithPolicies appends custom policies after the built-in stack and before the
// transport, in the order given.
func WithPolicies(policies ...pipeline.Policy) Option {
	return func(c *config) { c.custom = append(c.custom, policies...) }
}
