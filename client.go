// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package dexpace

import (
	"net/http"
	"time"

	"github.com/dexpace/go-sdk/auth"
	"github.com/dexpace/go-sdk/header"
	"github.com/dexpace/go-sdk/idempotency"
	"github.com/dexpace/go-sdk/logging"
	"github.com/dexpace/go-sdk/pipeline"
	"github.com/dexpace/go-sdk/retry"
	"github.com/dexpace/go-sdk/transport"
)

// Client is a thin handle around a configured [pipeline.Pipeline]. It is safe
// for concurrent use; create one with [New] and reuse it for the lifetime of the
// process.
type Client struct {
	pl pipeline.Pipeline
}

// New assembles a Client. Built-in policies are placed in stage order, outermost
// first:
//
//	client-identity → idempotency → retry → auth → date → logging → transport
//
// Retry wraps the inner stages, so auth re-runs (and may refresh its token) on
// every attempt; logging is innermost, so it records the request as sent.
// Idempotency-key stamping is on by default for POST (disable with
// WithoutIdempotency); set-date is opt-in (WithDate). Custom policies added with
// WithPolicies run just before the transport; use WithPolicyBefore /
// WithPolicyAfter to place a policy relative to a specific stage.
func New(opts ...Option) *Client {
	var cfg config
	for _, opt := range opts {
		opt(&cfg)
	}

	t := cfg.transport
	if t == nil {
		t = transport.New()
	}

	ua := cfg.userAgent
	if ua == "" {
		ua = userAgent
	}

	retryOpts := retry.Options{}
	if cfg.retry != nil {
		retryOpts = *cfg.retry
	}

	placements := []pipeline.Placement{
		pipeline.At(pipeline.StageClientIdentity, userAgentPolicy(ua)),
		pipeline.At(pipeline.StageRetry, retry.NewPolicy(retryOpts)),
	}

	if !cfg.noIdempotency {
		iopts := idempotency.Options{}
		if cfg.idempotency != nil {
			iopts = *cfg.idempotency
		}
		placements = append(placements,
			pipeline.At(pipeline.StageIdempotency, idempotency.NewPolicy(iopts)))
	}
	if cfg.credential != nil {
		placements = append(placements,
			pipeline.At(pipeline.StageAuth, auth.NewBearerTokenPolicy(cfg.credential, cfg.scopes...)))
	}
	if cfg.date {
		placements = append(placements, pipeline.At(pipeline.StageDate, datePolicy()))
	}
	if cfg.logging {
		placements = append(placements,
			pipeline.At(pipeline.StageLogging, logging.NewPolicy(logging.Options{Logger: cfg.logger})))
	}
	placements = append(placements, cfg.before...)
	placements = append(placements, cfg.after...)
	for _, p := range cfg.custom {
		// Custom WithPolicies land innermost, just before transport — anchored
		// after the innermost stage to preserve the previous behavior.
		placements = append(placements, pipeline.After(pipeline.StageLogging, p))
	}

	return &Client{pl: pipeline.NewStaged(t, placements...)}
}

// Do sends req through the pipeline and returns the response. The caller owns
// the response body and must close it.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	return c.pl.Do(req)
}

// Pipeline returns the underlying pipeline for advanced use (for example,
// embedding it in a higher-level pipeline or inspecting it in tests).
func (c *Client) Pipeline() pipeline.Pipeline {
	return c.pl
}

// datePolicy stamps the Date header in HTTP-date format (RFC 1123 with GMT, per
// RFC 7231) unless the caller already set one.
func datePolicy() pipeline.Policy {
	return pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		if req.Raw().Header.Get(header.Date) == "" {
			req.Raw().Header.Set(header.Date, time.Now().UTC().Format(http.TimeFormat))
		}
		return req.Next()
	})
}

// userAgentPolicy sets the User-Agent header unless the caller already provided
// one on the request.
func userAgentPolicy(ua string) pipeline.Policy {
	return pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		if req.Raw().Header.Get(header.UserAgent) == "" {
			req.Raw().Header.Set(header.UserAgent, ua)
		}
		return req.Next()
	})
}
