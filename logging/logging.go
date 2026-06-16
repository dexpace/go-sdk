// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package logging provides a pipeline policy that emits structured request and
// response records through log/slog. URLs are redacted so credentials carried in
// userinfo never reach the logs.
package logging

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/dexpace/go-sdk/pipeline"
	"github.com/dexpace/go-sdk/redact"
)

// Options configures the logging [Policy].
type Options struct {
	// Logger is the destination. When nil, slog.Default() is used.
	Logger *slog.Logger
	// Level is the level for request/response records. The zero value
	// (slog.LevelInfo) is used when unset. Failures are always logged at
	// slog.LevelError.
	Level slog.Level
	// Redactor renders URLs for the log records. When nil, redact.Default is used.
	Redactor *redact.Redactor
}

// Policy logs each request as it passes through the pipeline and the matching
// response (or error) on the way back, including the elapsed time. It implements
// pipeline.Policy.
//
// Place it where the granularity you want lives: below a retry policy it logs
// every attempt; above one it logs a single operation spanning all retries.
type Policy struct {
	logger   *slog.Logger
	level    slog.Level
	redactor *redact.Redactor
}

// NewPolicy returns a logging policy configured by opts.
func NewPolicy(opts Options) *Policy {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	r := opts.Redactor
	if r == nil {
		r = redact.Default
	}
	return &Policy{logger: logger, level: opts.Level, redactor: r}
}

// Do implements pipeline.Policy.
func (p *Policy) Do(req *pipeline.Request) (*http.Response, error) {
	raw := req.Raw()
	ctx := raw.Context()
	target := p.redactor.URL(raw.URL)
	start := time.Now()

	p.logger.LogAttrs(ctx, p.level, "http request",
		slog.String("method", raw.Method),
		slog.String("url", target),
	)

	resp, err := req.Next()
	elapsed := time.Since(start)

	if err != nil {
		p.logger.LogAttrs(ctx, slog.LevelError, "http request failed",
			slog.String("method", raw.Method),
			slog.String("url", target),
			slog.Duration("elapsed", elapsed),
			slog.String("error", err.Error()),
		)
		return resp, err
	}

	p.logger.LogAttrs(ctx, p.level, "http response",
		slog.String("method", raw.Method),
		slog.String("url", target),
		slog.Int("status", resp.StatusCode),
		slog.Duration("elapsed", elapsed),
	)
	return resp, nil
}
