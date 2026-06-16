// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package transport provides the default [pipeline.Transporter] for the SDK,
// backed by the standard library's net/http client. It is the terminal stage of
// a pipeline: it performs the actual network round-trip and applies no policy
// logic of its own.
package transport

import (
	"fmt"
	"net/http"
	"time"
)

// Transport is a pipeline.Transporter backed by an *http.Client.
type Transport struct {
	client *http.Client
}

// Option configures a [Transport].
type Option func(*config)

type config struct {
	client        *http.Client
	roundTripper  http.RoundTripper
	timeout       time.Duration
	checkRedirect func(req *http.Request, via []*http.Request) error
}

// WithClient supplies a fully configured *http.Client. When set, it takes
// precedence over [WithTimeout] and [WithRoundTripper].
func WithClient(c *http.Client) Option { return func(cfg *config) { cfg.client = c } }

// WithTimeout sets the per-request timeout on the default client. Ignored when
// [WithClient] is supplied. Prefer a per-request context deadline for
// finer-grained control.
func WithTimeout(d time.Duration) Option { return func(cfg *config) { cfg.timeout = d } }

// WithRoundTripper sets a custom http.RoundTripper (for example a tuned
// *http.Transport, or a stub in tests) on the default client. Ignored when
// [WithClient] is supplied.
func WithRoundTripper(rt http.RoundTripper) Option {
	return func(cfg *config) { cfg.roundTripper = rt }
}

// WithMaxRedirects caps how many redirects the default client follows. A value
// of 0 stops following redirects and returns the redirect response itself.
// Ignored when [WithClient] is supplied. net/http already strips sensitive
// headers (Authorization, Cookie) on cross-origin redirects.
func WithMaxRedirects(n int) Option {
	return func(cfg *config) {
		cfg.checkRedirect = func(_ *http.Request, via []*http.Request) error {
			if n <= 0 {
				return http.ErrUseLastResponse
			}
			if len(via) >= n {
				return fmt.Errorf("transport: stopped after %d redirects", n)
			}
			return nil
		}
	}
}

// WithRedirectPolicy sets a custom redirect policy on the default client,
// mirroring http.Client.CheckRedirect: return nil to follow,
// http.ErrUseLastResponse to stop and return the last response, or any other
// error to fail the call. Ignored when [WithClient] is supplied.
func WithRedirectPolicy(fn func(req *http.Request, via []*http.Request) error) Option {
	return func(cfg *config) { cfg.checkRedirect = fn }
}

// New builds a Transport. With no options it uses an *http.Client whose
// RoundTripper is cloned from http.DefaultTransport with slightly larger
// connection-pool limits suited to SDK workloads.
func New(opts ...Option) *Transport {
	var cfg config
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.client != nil {
		return &Transport{client: cfg.client}
	}
	rt := cfg.roundTripper
	if rt == nil {
		rt = defaultRoundTripper()
	}
	return &Transport{client: &http.Client{
		Transport:     rt,
		Timeout:       cfg.timeout,
		CheckRedirect: cfg.checkRedirect,
	}}
}

// Do performs the HTTP round-trip. It satisfies pipeline.Transporter.
func (t *Transport) Do(req *http.Request) (*http.Response, error) {
	return t.client.Do(req) //nolint:gosec // G704: this is the SDK's HTTP transport; issuing the caller's own request is its sole purpose.
}

// defaultRoundTripper clones http.DefaultTransport so global state is untouched,
// then raises the idle-connection limits that matter for SDKs talking to a
// single API host.
func defaultRoundTripper() http.RoundTripper {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return http.DefaultTransport
	}
	rt := base.Clone()
	rt.MaxIdleConns = 100
	rt.MaxIdleConnsPerHost = 100
	rt.IdleConnTimeout = 90 * time.Second
	return rt
}
