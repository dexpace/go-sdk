// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package dexpace

import (
	"log/slog"

	"github.com/dexpace/go-sdk/auth"
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
