// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package dexpace

import (
	"log/slog"

	"github.com/dexpace/go-sdk/auth"
	cfgpkg "github.com/dexpace/go-sdk/config"
	"github.com/dexpace/go-sdk/idempotency"
	"github.com/dexpace/go-sdk/instrumentation"
	"github.com/dexpace/go-sdk/pipeline"
	"github.com/dexpace/go-sdk/retry"
)

// Option configures a [Client] built by [New]. Options are applied in the order
// given; later options override earlier ones for the same setting.
type Option func(*config)

type apiKeyConfig struct {
	header string
	key    string
	set    bool
}

type config struct {
	transport  pipeline.Transporter
	retry      *retry.Options
	credential auth.TokenCredential
	scopes     []string
	tokenCache auth.TokenCache
	basicAuth  *auth.BasicCredential
	apiKey     apiKeyConfig
	digestAuth *auth.BasicCredential
	logger     *slog.Logger
	logging    bool
	date       bool
	userAgent  string
	custom     []pipeline.Policy

	noIdempotency bool
	idempotency   *idempotency.Options
	before        []pipeline.Placement
	after         []pipeline.Placement
	errorsEnabled bool

	tracer      instrumentation.Tracer
	meter       instrumentation.Meter
	redactAllow []string

	cfgSource *cfgpkg.Config
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

// WithTokenCache shares cache across the bearer-token policy installed by
// WithCredential, so multiple clients can reuse cached tokens. A nil cache or no
// credential means the default per-client in-memory cache is used.
func WithTokenCache(cache auth.TokenCache) Option {
	return func(c *config) { c.tokenCache = cache }
}

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

// WithDigestAuth authenticates requests with HTTP Digest Access Authentication
// (RFC 7616). Like all credential policies it requires HTTPS. Precedence when
// multiple auth options are set: WithCredential, WithBasicAuth, WithAPIKey, then
// WithDigestAuth.
func WithDigestAuth(username, password string) Option {
	return func(c *config) {
		c.digestAuth = &auth.BasicCredential{Username: username, Password: password}
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

// WithErrors enables the typed error model. With it, Client.Do returns a
// *httperr.ResponseError for a non-2xx response and a *httperr.TransportError
// for a request that never produced a response (context cancellation/deadline
// errors are returned unchanged). Off by default: without it, Do mirrors
// http.Client.Do — a non-2xx status is not an error, and transport failures
// surface as raw net/http errors.
func WithErrors() Option {
	return func(c *config) { c.errorsEnabled = true }
}

// WithTracing installs a tracing policy that records a span around each request
// attempt using tracer, injecting a W3C traceparent header. A nil tracer is
// ignored (no policy installed). Off by default.
func WithTracing(tracer instrumentation.Tracer) Option {
	return func(c *config) { c.tracer = tracer }
}

// WithMetrics installs a metrics policy that records request duration and
// in-flight count using meter. A nil meter is ignored (no policy installed). Off
// by default.
func WithMetrics(meter instrumentation.Meter) Option {
	return func(c *config) { c.meter = meter }
}

// WithConfig supplies client defaults from cfg for any setting the caller did not
// set explicitly: the User-Agent (DEXPACE_USER_AGENT), retry count and base delay
// (DEXPACE_MAX_RETRIES, DEXPACE_RETRY_BASE_DELAY), and the default-transport
// timeout (DEXPACE_HTTP_TIMEOUT, applied only when no transport is supplied via
// WithTransport). Explicit options always win, regardless of option order. A nil
// cfg is a no-op.
func WithConfig(cfg *cfgpkg.Config) Option {
	return func(c *config) { c.cfgSource = cfg }
}

// WithRedactionAllowlist preserves the values of the named query parameters in
// redacted URLs (logs and traces); all other query values are redacted. Applies
// to the logging and tracing policies.
func WithRedactionAllowlist(params ...string) Option {
	return func(c *config) { c.redactAllow = params }
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
