// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package pipeline

import (
	"errors"
	"net/http"
)

// Pipeline is an immutable, ordered chain of [Policy] values terminated by a
// [Transporter]. Build one with [New] and run requests through it with
// [Pipeline.Do]. The zero value is not usable.
//
// Pipeline is safe for concurrent use: it holds no per-request state, and each
// call to Do allocates a fresh [Request] to carry state through the chain.
type Pipeline struct {
	policies []Policy
}

// New builds a Pipeline that runs policies in order and finishes by handing the
// request to transport. Order is significant: an earlier policy wraps the later
// ones, so a retry policy placed before an auth policy re-authenticates on every
// attempt, whereas the reverse order signs once and retries the signed request.
//
// transport must be non-nil; passing nil is a programming error and panics.
func New(transport Transporter, policies ...Policy) Pipeline {
	if transport == nil {
		panic("pipeline: New requires a non-nil Transporter")
	}
	all := make([]Policy, 0, len(policies)+1)
	all = append(all, policies...)
	all = append(all, transportPolicy{transport: transport})
	return Pipeline{policies: all}
}

// Do sends req through the pipeline and returns the transport's response. The
// caller owns the returned response body and must close it.
func (p Pipeline) Do(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("pipeline: Do called with a nil request")
	}
	if len(p.policies) == 0 {
		return nil, errors.New("pipeline: Do called on a zero-value Pipeline; build it with New")
	}
	r := &Request{req: req, policies: p.policies, values: make(map[any]any)}
	return r.Next()
}

// transportPolicy is the implicit terminal policy: it performs the round-trip
// and never calls Next.
type transportPolicy struct {
	transport Transporter
}

func (t transportPolicy) Do(req *Request) (*http.Response, error) {
	return t.transport.Do(req.Raw())
}
