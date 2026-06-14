// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package retry provides a pipeline policy that transparently retries failed
// HTTP attempts using exponential backoff with full jitter, honouring a
// server-supplied Retry-After header.
package retry

import (
	"context"
	"errors"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"

	"github.com/dexpace/go-sdk/header"
	"github.com/dexpace/go-sdk/pipeline"
)

// Default backoff parameters, applied by [Options.withDefaults] when a field is
// left at its zero value.
const (
	defaultMaxRetries = 3
	defaultBaseDelay  = 800 * time.Millisecond
	defaultMaxDelay   = 60 * time.Second
)

// drainLimit caps how much of a to-be-discarded response body is read before
// closing, so a connection can be reused without buffering an unbounded body.
const drainLimit = 4 << 10

// Options configures the retry [Policy]. The zero value is valid and yields the
// documented defaults.
type Options struct {
	// MaxRetries is the number of retries after the initial attempt. Zero selects
	// the default (3); a negative value disables retries entirely.
	MaxRetries int

	// BaseDelay is the first backoff interval; it doubles each retry up to
	// MaxDelay. Zero or negative selects the default (800ms).
	BaseDelay time.Duration

	// MaxDelay caps the backoff interval. Zero or negative selects the default
	// (60s).
	MaxDelay time.Duration

	// StatusCodes lists response status codes that trigger a retry. When nil, a
	// default set of transient codes is used: 408, 429, 500, 502, 503, 504.
	StatusCodes []int
}

func (o Options) withDefaults() Options {
	if o.MaxRetries == 0 {
		o.MaxRetries = defaultMaxRetries
	}
	if o.BaseDelay <= 0 {
		o.BaseDelay = defaultBaseDelay
	}
	if o.MaxDelay <= 0 {
		o.MaxDelay = defaultMaxDelay
	}
	if o.StatusCodes == nil {
		o.StatusCodes = defaultStatusCodes()
	}
	return o
}

func defaultStatusCodes() []int {
	return []int{
		http.StatusRequestTimeout,      // 408
		http.StatusTooManyRequests,     // 429
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout,      // 504
	}
}

// Policy retries transient failures. It implements pipeline.Policy.
//
// On a transport error (the request never produced a response) the policy
// retries only requests that are safe to repeat: methods that are idempotent by
// definition (GET, HEAD, OPTIONS, TRACE, PUT, DELETE), or any request carrying an
// idempotency key (see package idempotency). On a response, the policy retries
// the configured status codes.
type Policy struct {
	opts Options
}

// NewPolicy returns a retry policy configured by opts.
func NewPolicy(opts Options) *Policy {
	return &Policy{opts: opts.withDefaults()}
}

// Do implements pipeline.Policy.
func (p *Policy) Do(req *pipeline.Request) (*http.Response, error) {
	ctx := req.Raw().Context()

	var (
		resp *http.Response
		err  error
	)
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			if rewindErr := req.RewindBody(); rewindErr != nil {
				return nil, rewindErr
			}
		}

		resp, err = req.Next()
		if !p.shouldRetry(attempt, req, resp, err) {
			return resp, err
		}

		delay := p.backoff(attempt, resp)
		drainAndClose(resp)

		if waitErr := sleep(ctx, delay); waitErr != nil {
			if err != nil {
				return nil, err
			}
			return nil, waitErr
		}
	}
}

func (p *Policy) shouldRetry(attempt int, req *pipeline.Request, resp *http.Response, err error) bool {
	if attempt >= p.opts.MaxRetries {
		return false
	}
	if err != nil {
		return retryableErr(err) && retrySafe(req)
	}
	return p.retryableStatus(resp.StatusCode)
}

// retrySafe reports whether a request may be re-sent after a transport error.
// Safe and idempotent methods always qualify; other methods (POST, PATCH) only
// qualify when an idempotency key makes the repeat safe.
func retrySafe(req *pipeline.Request) bool {
	switch req.Raw().Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions,
		http.MethodTrace, http.MethodPut, http.MethodDelete:
		return true
	}
	if pipeline.IsIdempotent(req) {
		return true
	}
	return req.Raw().Header.Get(idempotencyKeyHeader) != ""
}

const idempotencyKeyHeader = "Idempotency-Key"

func (p *Policy) retryableStatus(code int) bool {
	for _, c := range p.opts.StatusCodes {
		if c == code {
			return true
		}
	}
	return false
}

// retryableErr reports whether a transport error is worth retrying. Context
// cancellation and deadline expiry are terminal and never retried.
func retryableErr(err error) bool {
	switch {
	case err == nil:
		return false
	case isContextErr(err):
		return false
	default:
		// The request never produced a response, so a connection/timeout error is
		// generally safe to retry for idempotent methods.
		return true
	}
}

func isContextErr(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// backoff returns the wait before the next attempt. A server Retry-After header
// wins; otherwise exponential backoff with full jitter, capped at MaxDelay.
func (p *Policy) backoff(attempt int, resp *http.Response) time.Duration {
	if resp != nil {
		if d, ok := retryAfter(resp); ok {
			return min(d, p.opts.MaxDelay)
		}
	}
	exp := float64(p.opts.BaseDelay) * math.Pow(2, float64(attempt))
	capped := math.Min(exp, float64(p.opts.MaxDelay))
	return time.Duration(rand.Float64() * capped) //nolint:gosec // jitter, not security-sensitive
}

// retryAfter parses a Retry-After header expressed either as delay-seconds or as
// an HTTP date.
func retryAfter(resp *http.Response) (time.Duration, bool) {
	v := resp.Header.Get(header.RetryAfter)
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			return 0, false
		}
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(v); err == nil {
		return max(time.Until(t), 0), true
	}
	return 0, false
}

func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, drainLimit))
	_ = resp.Body.Close()
}

// sleep waits for d, returning early with the context error if ctx is cancelled.
func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
