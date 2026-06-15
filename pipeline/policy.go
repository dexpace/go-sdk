// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package pipeline

import (
	"errors"
	"fmt"
	"net/http"
)

// Transporter is the terminal stage of a [Pipeline]: it sends an HTTP request
// over the wire and returns the response. It is the narrowest possible seam —
// a single method mirroring http.Client.Do — so that any HTTP client can plug
// in. Package transport provides the default net/http-backed implementation.
type Transporter interface {
	Do(req *http.Request) (*http.Response, error)
}

// Policy observes or mutates a request as it flows through a [Pipeline] and then
// hands control downstream by calling [Request.Next]. Implementations must be
// safe for concurrent use, since one Policy instance serves every request that
// runs through the pipeline it belongs to.
type Policy interface {
	Do(req *Request) (*http.Response, error)
}

// PolicyFunc adapts an ordinary function to the [Policy] interface.
type PolicyFunc func(req *Request) (*http.Response, error)

// Do calls f(req). It implements [Policy].
func (f PolicyFunc) Do(req *Request) (*http.Response, error) { return f(req) }

// Request carries an *http.Request through the policy chain. It is created by
// [Pipeline.Do] and threaded into each [Policy]. A policy advances the chain
// with [Request.Next] and may call it repeatedly — the retry policy relies on
// this — provided the body is rewound with [Request.RewindBody] between attempts.
//
// A Request is not safe for concurrent use; it belongs to the single goroutine
// driving one HTTP call.
type Request struct {
	req      *http.Request
	policies []Policy
	values   map[any]any
}

// Raw returns the underlying *http.Request. Policies mutate the request (set
// headers, swap the body) directly through this handle.
func (r *Request) Raw() *http.Request { return r.req }

// Next invokes the next policy in the chain and returns its result. The terminal
// policy performs the transport round-trip. Calling Next more than once re-runs
// every downstream policy from scratch (used to implement retries); rewind the
// body with [Request.RewindBody] beforehand when the request carries a payload.
func (r *Request) Next() (*http.Response, error) {
	if len(r.policies) == 0 {
		return nil, errors.New("pipeline: Next called with no remaining policies (a policy must terminate the chain)")
	}
	next := r.policies[0]
	child := &Request{req: r.req, policies: r.policies[1:], values: r.values}
	return next.Do(child)
}

// RewindBody resets the underlying request body to the beginning so the request
// can be sent again. It relies on http.Request.GetBody, which net/http populates
// automatically for the common in-memory body types (bytes.Reader,
// bytes.Buffer, strings.Reader). It is a no-op for bodyless requests.
//
// It returns an error when the body cannot be replayed; callers that need
// retryable streaming bodies must buffer them and set GetBody themselves.
func (r *Request) RewindBody() error {
	raw := r.req
	if raw.Body == nil || raw.Body == http.NoBody {
		return nil
	}
	if raw.GetBody == nil {
		return errors.New("pipeline: request body is not replayable (GetBody is nil); buffer the body before sending if retries are needed")
	}
	body, err := raw.GetBody()
	if err != nil {
		return fmt.Errorf("pipeline: rewind body: %w", err)
	}
	raw.Body = body
	return nil
}

// SetValue stores a value on the request for a downstream policy to read with
// [Request.Value]. Use an unexported key type to avoid collisions between
// packages, mirroring the convention for context values.
func (r *Request) SetValue(key, value any) {
	if r.values == nil {
		r.values = make(map[any]any)
	}
	r.values[key] = value
}

// Value returns the value previously stored under key, or nil if absent.
func (r *Request) Value(key any) any {
	if r.values == nil {
		return nil
	}
	return r.values[key]
}
