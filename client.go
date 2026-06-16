// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package dexpace

import (
	"net/http"
	"time"

	"github.com/dexpace/go-sdk/auth"
	"github.com/dexpace/go-sdk/header"
	"github.com/dexpace/go-sdk/httperr"
	"github.com/dexpace/go-sdk/idempotency"
	"github.com/dexpace/go-sdk/instrumentation"
	"github.com/dexpace/go-sdk/logging"
	"github.com/dexpace/go-sdk/pipeline"
	"github.com/dexpace/go-sdk/redact"
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
//	client-identity → idempotency → retry → auth → date → [tracing] → [metrics] → logging → transport
//
// When WithErrors is supplied, an errors stage is prepended as the outermost
// policy, mapping the final result to the typed error model.
// When WithTracing or WithMetrics is supplied, a tracing or metrics stage is
// installed at StageTracing or StageMetrics (inside retry).
//
// Idempotency wraps retry, so a single key is minted once per logical call and
// reused across attempts; retry in turn wraps auth and logging, so auth re-runs
// (and may refresh its token) on every attempt and logging — innermost — records
// the request as actually sent.
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

	redactor := redact.Default
	if len(cfg.redactAllow) > 0 {
		redactor = redact.New(cfg.redactAllow...)
	}

	placements := []pipeline.Placement{
		pipeline.At(pipeline.StageClientIdentity, userAgentPolicy(ua)),
		pipeline.At(pipeline.StageRetry, retry.NewPolicy(retryOpts)),
	}

	if cfg.errorsEnabled {
		placements = append(placements, pipeline.At(pipeline.StageErrors, errorsPolicy()))
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
			pipeline.At(pipeline.StageLogging, logging.NewPolicy(logging.Options{Logger: cfg.logger, Redactor: redactor})))
	}
	if cfg.tracer != nil {
		placements = append(placements,
			pipeline.At(pipeline.StageTracing, instrumentation.NewTracingPolicy(cfg.tracer, redactor)))
	}
	if cfg.meter != nil {
		placements = append(placements,
			pipeline.At(pipeline.StageMetrics, instrumentation.NewMetricsPolicy(cfg.meter)))
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

// errorsPolicy maps the final result of the chain to the typed error model: a
// transport failure becomes a *httperr.TransportError (context errors pass
// through unchanged), and a non-2xx response becomes a *httperr.ResponseError.
// Callers place it at [pipeline.StageErrors], the outermost stage, so retry
// still operates on raw responses.
func errorsPolicy() pipeline.Policy {
	return pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		resp, err := req.Next()
		if err != nil {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			return nil, httperr.FromError(err, req.Raw())
		}
		if rerr := httperr.FromResponse(resp); rerr != nil {
			return resp, rerr
		}
		return resp, nil
	})
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
