// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package dexpace

import (
	"net/http"

	"github.com/dexpace/go-sdk/auth"
	"github.com/dexpace/go-sdk/header"
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

// New assembles a Client. With no options it uses the default net/http transport
// and a default retry policy. Options compose the standard policy stack in this
// fixed order, outermost first:
//
//	user-agent → retry → logging (if enabled) → auth (if credential set) → custom → transport
//
// Because retry wraps the inner policies, auth re-runs (and may refresh its
// token) on every attempt, and logging — placed inside retry — records each
// attempt separately.
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

	policies := make([]pipeline.Policy, 0, 5+len(cfg.custom))
	policies = append(policies, userAgentPolicy(ua))
	policies = append(policies, retry.NewPolicy(retryOpts))
	if cfg.logging {
		policies = append(policies, logging.NewPolicy(logging.Options{Logger: cfg.logger}))
	}
	if cfg.credential != nil {
		policies = append(policies, auth.NewBearerTokenPolicy(cfg.credential, cfg.scopes...))
	}
	policies = append(policies, cfg.custom...)

	return &Client{pl: pipeline.New(t, policies...)}
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
