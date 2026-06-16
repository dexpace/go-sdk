// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package dexpace_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	dexpace "github.com/dexpace/go-sdk"
	"github.com/dexpace/go-sdk/auth"
	"github.com/dexpace/go-sdk/config"
	"github.com/dexpace/go-sdk/httperr"
	"github.com/dexpace/go-sdk/instrumentation"
	"github.com/dexpace/go-sdk/pipeline"
	"github.com/dexpace/go-sdk/retry"
)

type transporterFunc func(*http.Request) (*http.Response, error)

func (f transporterFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

func captureTransport(captured **http.Request) transporterFunc {
	return func(r *http.Request) (*http.Response, error) {
		*captured = r
		return &http.Response{StatusCode: 200, Body: http.NoBody, Request: r}, nil
	}
}

func TestWithDateStampsHeader(t *testing.T) {
	t.Parallel()

	var captured *http.Request
	c := dexpace.New(
		dexpace.WithTransport(captureTransport(&captured)),
		dexpace.WithDate(),
	)
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if captured.Header.Get("Date") == "" {
		t.Fatal("Date header not set with WithDate()")
	}
}

func TestDateOffByDefault(t *testing.T) {
	t.Parallel()

	var captured *http.Request
	c := dexpace.New(dexpace.WithTransport(captureTransport(&captured)))
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if captured.Header.Get("Date") != "" {
		t.Fatal("Date header set without WithDate()")
	}
}

func TestWithDateKeepsCallerDate(t *testing.T) {
	t.Parallel()

	const callerDate = "Sun, 01 Jan 2023 00:00:00 GMT"
	var captured *http.Request
	c := dexpace.New(
		dexpace.WithTransport(captureTransport(&captured)),
		dexpace.WithDate(),
	)
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	req.Header.Set("Date", callerDate)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if got := captured.Header.Get("Date"); got != callerDate {
		t.Fatalf("Date = %q, want caller value %q", got, callerDate)
	}
}

func TestIdempotencyOnByDefaultForPost(t *testing.T) {
	t.Parallel()

	var captured *http.Request
	c := dexpace.New(dexpace.WithTransport(captureTransport(&captured)))
	req, _ := http.NewRequest(http.MethodPost, "https://example.test/", strings.NewReader("x"))
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if captured.Header.Get("Idempotency-Key") == "" {
		t.Fatal("Idempotency-Key not set by default on POST")
	}
}

func TestWithoutIdempotency(t *testing.T) {
	t.Parallel()

	var captured *http.Request
	c := dexpace.New(
		dexpace.WithTransport(captureTransport(&captured)),
		dexpace.WithoutIdempotency(),
	)
	req, _ := http.NewRequest(http.MethodPost, "https://example.test/", strings.NewReader("x"))
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if captured.Header.Get("Idempotency-Key") != "" {
		t.Fatal("Idempotency-Key set despite WithoutIdempotency()")
	}
}

func TestWithPolicyBeforeAndAfterRun(t *testing.T) {
	t.Parallel()

	var ran []string
	mk := func(name string) pipeline.Policy {
		return pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
			ran = append(ran, name)
			return req.Next()
		})
	}
	var captured *http.Request
	c := dexpace.New(
		dexpace.WithTransport(captureTransport(&captured)),
		dexpace.WithPolicyBefore(pipeline.StageRetry, mk("before-retry")),
		dexpace.WithPolicyAfter(pipeline.StageAuth, mk("after-auth")),
	)
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if len(ran) != 2 || ran[0] != "before-retry" || ran[1] != "after-auth" {
		t.Fatalf("custom policies ran = %v, want [before-retry after-auth]", ran)
	}
}

func TestCustomPolicyPositionRelativeToRetry(t *testing.T) {
	t.Parallel()

	var outside, inside, attempts int
	before := pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		outside++
		return req.Next()
	})
	after := pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		inside++
		return req.Next()
	})
	failing := transporterFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		return nil, errors.New("dial tcp: connection refused")
	})

	c := dexpace.New(
		dexpace.WithTransport(failing),
		dexpace.WithRetry(retry.Options{MaxRetries: 2, BaseDelay: 1, MaxDelay: 1}),
		dexpace.WithPolicyBefore(pipeline.StageRetry, before),
		dexpace.WithPolicyAfter(pipeline.StageRetry, after),
	)
	// GET so the retry policy treats transport errors as retry-safe.
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	_, _ = c.Do(req)

	if attempts != 3 {
		t.Fatalf("transport attempts = %d, want 3 (initial + 2 retries)", attempts)
	}
	if outside != 1 {
		t.Fatalf("before-retry policy ran %d times, want 1 (outside retry)", outside)
	}
	if inside != 3 {
		t.Fatalf("after-retry policy ran %d times, want 3 (inside retry)", inside)
	}
}

// statusTransport returns a fresh response with the given status code each call.
func statusTransport(code int, calls *int) transporterFunc {
	return func(req *http.Request) (*http.Response, error) {
		if calls != nil {
			*calls++
		}
		return &http.Response{
			StatusCode: code,
			Status:     fmt.Sprintf("%d %s", code, http.StatusText(code)),
			Body:       http.NoBody,
			Request:    req,
		}, nil
	}
}

// errTransport always fails with a transport error.
func errTransport(err error) transporterFunc {
	return func(*http.Request) (*http.Response, error) { return nil, err }
}

func TestErrorsOffByDefault(t *testing.T) {
	t.Parallel()

	c := dexpace.New(dexpace.WithTransport(statusTransport(http.StatusNotFound, nil)))
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do = %v, want nil error by default", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}

	cause := errors.New("connection refused")
	c2 := dexpace.New(
		dexpace.WithTransport(errTransport(cause)),
		dexpace.WithRetry(retry.Options{MaxRetries: -1}),
	)
	_, err2 := c2.Do(req)
	var te *httperr.TransportError
	if errors.As(err2, &te) {
		t.Fatal("transport error should be raw without WithErrors()")
	}
	if !errors.Is(err2, cause) {
		t.Fatalf("err = %v, want the raw cause", err2)
	}
}

func TestWithErrorsConvertsStatusError(t *testing.T) {
	t.Parallel()

	c := dexpace.New(
		dexpace.WithTransport(statusTransport(http.StatusNotFound, nil)),
		dexpace.WithErrors(),
	)
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := c.Do(req)
	if resp != nil {
		t.Cleanup(func() { _ = resp.Body.Close() })
	}

	var rerr *httperr.ResponseError
	if !errors.As(err, &rerr) {
		t.Fatalf("err = %T, want *httperr.ResponseError", err)
	}
	if rerr.StatusCode != http.StatusNotFound {
		t.Fatalf("StatusCode = %d, want 404", rerr.StatusCode)
	}
}

func TestWithErrorsWrapsTransportError(t *testing.T) {
	t.Parallel()

	cause := errors.New("dial tcp: connection refused")
	c := dexpace.New(
		dexpace.WithTransport(errTransport(cause)),
		dexpace.WithErrors(),
		dexpace.WithRetry(retry.Options{MaxRetries: -1}),
	)
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	_, err := c.Do(req)

	var te *httperr.TransportError
	if !errors.As(err, &te) {
		t.Fatalf("err = %T, want *httperr.TransportError", err)
	}
	if !errors.Is(err, cause) {
		t.Fatal("Unwrap must reach the underlying cause")
	}
}

func TestWithErrorsRetryStillSeesRawResponse(t *testing.T) {
	t.Parallel()

	var calls int
	c := dexpace.New(
		dexpace.WithTransport(statusTransport(http.StatusServiceUnavailable, &calls)),
		dexpace.WithErrors(),
		dexpace.WithRetry(retry.Options{MaxRetries: 2, BaseDelay: 1, MaxDelay: 1}),
	)
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := c.Do(req)
	if resp != nil {
		t.Cleanup(func() { _ = resp.Body.Close() })
	}

	if calls != 3 {
		t.Fatalf("transport calls = %d, want 3 (retry saw raw 503s)", calls)
	}
	var rerr *httperr.ResponseError
	if !errors.As(err, &rerr) || rerr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("final err = %v, want *ResponseError with 503", err)
	}
}

func TestWithErrorsPassesThroughContextCancellation(t *testing.T) {
	t.Parallel()

	// A transport that fails with a context.Canceled-wrapped error.
	canceled := errTransport(&url.Error{Op: "Get", URL: "https://example.test/", Err: context.Canceled})
	c := dexpace.New(
		dexpace.WithTransport(canceled),
		dexpace.WithErrors(),
		dexpace.WithRetry(retry.Options{MaxRetries: -1}),
	)
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	_, err := c.Do(req)

	var te *httperr.TransportError
	if errors.As(err, &te) {
		t.Fatal("context cancellation must not be wrapped as *TransportError")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled to pass through", err)
	}
}

// closeTrackingBody records whether Close was called.
type closeTrackingBody struct {
	closed *bool
}

func (closeTrackingBody) Read([]byte) (int, error) { return 0, io.EOF }
func (b closeTrackingBody) Close() error           { *b.closed = true; return nil }

func TestWithErrorsClosesBodyOnTransportError(t *testing.T) {
	t.Parallel()

	var closed bool
	cause := errors.New("partial response then failure")
	// A transport that returns BOTH a non-nil response and a non-nil error.
	tr := transporterFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       closeTrackingBody{closed: &closed},
			Request:    req,
		}, cause
	})
	c := dexpace.New(
		dexpace.WithTransport(tr),
		dexpace.WithErrors(),
		dexpace.WithRetry(retry.Options{MaxRetries: -1}),
	)
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := c.Do(req)

	if resp != nil {
		t.Fatalf("resp = %v, want nil on transport error", resp)
	}
	var te *httperr.TransportError
	if !errors.As(err, &te) {
		t.Fatalf("err = %T, want *httperr.TransportError", err)
	}
	if !closed {
		t.Fatal("response body was not closed on the transport-error path")
	}
}

type spySpan struct{}

func (spySpan) SetAttributes(...instrumentation.Attr) {}
func (spySpan) RecordError(error)                     {}
func (spySpan) End()                                  {}
func (spySpan) Context() instrumentation.SpanContext  { return instrumentation.SpanContext{} }

type spyTracer struct{ started bool }

func (s *spyTracer) StartSpan(ctx context.Context, _ string) (context.Context, instrumentation.Span) {
	s.started = true
	return ctx, spySpan{}
}

type spyHist struct{ m *spyMeter }

func (h spyHist) Record(context.Context, float64, ...instrumentation.Attr) { h.m.recorded = true }

type spyUD struct{}

func (spyUD) Add(context.Context, int64, ...instrumentation.Attr) {}

type spyMeter struct{ recorded bool }

func (m *spyMeter) Histogram(string) instrumentation.Histogram         { return spyHist{m} }
func (m *spyMeter) UpDownCounter(string) instrumentation.UpDownCounter { return spyUD{} }

func TestWithTracingInstallsPolicy(t *testing.T) {
	t.Parallel()

	tr := &spyTracer{}
	c := dexpace.New(dexpace.WithTransport(statusTransport(http.StatusOK, nil)), dexpace.WithTracing(tr))
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if !tr.started {
		t.Fatal("tracer was not invoked; tracing policy not installed")
	}
}

func TestWithMetricsInstallsPolicy(t *testing.T) {
	t.Parallel()

	m := &spyMeter{}
	c := dexpace.New(dexpace.WithTransport(statusTransport(http.StatusOK, nil)), dexpace.WithMetrics(m))
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if !m.recorded {
		t.Fatal("meter was not invoked; metrics policy not installed")
	}
}

func TestObservabilityOffByDefault(t *testing.T) {
	t.Parallel()

	var captured *http.Request
	c := dexpace.New(dexpace.WithTransport(captureTransport(&captured)))
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if got := captured.Header.Get("Traceparent"); got != "" {
		t.Fatalf("Traceparent = %q, want empty when WithTracing is not used", got)
	}
}

func TestWithRedactionAllowlistAppliesToLogging(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	c := dexpace.New(
		dexpace.WithTransport(statusTransport(http.StatusOK, nil)),
		dexpace.WithLogging(logger),
		dexpace.WithRedactionAllowlist("page"),
	)
	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/x?token=secret&page=2", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	out := buf.String()
	if strings.Contains(out, "secret") {
		t.Fatalf("log leaked the query secret: %s", out)
	}
	if !strings.Contains(out, "token=REDACTED") {
		t.Fatalf("log should redact token: %s", out)
	}
	if !strings.Contains(out, "page=2") {
		t.Fatalf("allowlisted page should be preserved: %s", out)
	}
}

func TestWithConfigSetsUserAgentFromEnv(t *testing.T) {
	t.Setenv("DEXPACE_USER_AGENT", "custom-agent/9")

	var captured *http.Request
	c := dexpace.New(
		dexpace.WithTransport(captureTransport(&captured)),
		dexpace.WithConfig(config.New()),
	)
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if got := captured.Header.Get("User-Agent"); got != "custom-agent/9" {
		t.Fatalf("User-Agent = %q, want custom-agent/9", got)
	}
}

func TestExplicitUserAgentBeatsConfig(t *testing.T) {
	t.Setenv("DEXPACE_USER_AGENT", "from-env")

	var captured *http.Request
	c := dexpace.New(
		dexpace.WithTransport(captureTransport(&captured)),
		dexpace.WithConfig(config.New()),
		dexpace.WithUserAgent("explicit/1"),
	)
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if got := captured.Header.Get("User-Agent"); got != "explicit/1" {
		t.Fatalf("User-Agent = %q, want explicit/1 (explicit beats config)", got)
	}
}

func TestWithConfigSetsRetriesFromEnv(t *testing.T) {
	t.Setenv("DEXPACE_MAX_RETRIES", "2")
	t.Setenv("DEXPACE_RETRY_BASE_DELAY", "1ns")

	var calls int
	c := dexpace.New(
		dexpace.WithTransport(statusTransport(http.StatusServiceUnavailable, &calls)),
		dexpace.WithConfig(config.New()),
	)
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if calls != 3 {
		t.Fatalf("transport calls = %d, want 3", calls)
	}
}

func TestWithConfigNilIsNoOp(t *testing.T) {
	t.Parallel()

	var captured *http.Request
	c := dexpace.New(
		dexpace.WithTransport(captureTransport(&captured)),
		dexpace.WithConfig(nil),
	)
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if ua := captured.Header.Get("User-Agent"); ua == "" {
		t.Fatal("expected the default User-Agent with WithConfig(nil)")
	}
}

type capturingSpan struct{ attrs []instrumentation.Attr }

func (s *capturingSpan) SetAttributes(a ...instrumentation.Attr) { s.attrs = append(s.attrs, a...) }
func (s *capturingSpan) RecordError(error)                       {}
func (s *capturingSpan) End()                                    {}
func (s *capturingSpan) Context() instrumentation.SpanContext    { return instrumentation.SpanContext{} }

type capturingTracer struct{ span *capturingSpan }

func (tr *capturingTracer) StartSpan(ctx context.Context, _ string) (context.Context, instrumentation.Span) {
	return ctx, tr.span
}

func TestWithRedactionAllowlistAppliesToTracing(t *testing.T) {
	t.Parallel()

	span := &capturingSpan{}
	tr := &capturingTracer{span: span}
	c := dexpace.New(
		dexpace.WithTransport(statusTransport(http.StatusOK, nil)),
		dexpace.WithTracing(tr),
		dexpace.WithRedactionAllowlist("page"),
	)
	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/x?token=secret&page=2", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	var urlFull string
	for _, a := range span.attrs {
		if a.Key == "url.full" {
			urlFull, _ = a.Value.(string)
		}
	}
	if strings.Contains(urlFull, "secret") {
		t.Fatalf("url.full leaked secret: %q", urlFull)
	}
	if !strings.Contains(urlFull, "token=REDACTED") || !strings.Contains(urlFull, "page=2") {
		t.Fatalf("url.full = %q, want token redacted and page preserved", urlFull)
	}
}

func TestWithConfigZeroMaxRetriesDisablesRetries(t *testing.T) {
	t.Setenv("DEXPACE_MAX_RETRIES", "0")

	var calls int
	c := dexpace.New(
		dexpace.WithTransport(statusTransport(http.StatusServiceUnavailable, &calls)),
		dexpace.WithConfig(config.New()),
	)
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if calls != 1 {
		t.Fatalf("transport calls = %d, want 1 (retries disabled by env)", calls)
	}
}

func TestWithBasicAuth(t *testing.T) {
	t.Parallel()

	var captured *http.Request
	c := dexpace.New(
		dexpace.WithTransport(captureTransport(&captured)),
		dexpace.WithBasicAuth("alice", "s3cr3t"),
	)
	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	u, p, ok := captured.BasicAuth()
	if !ok || u != "alice" || p != "s3cr3t" {
		t.Fatalf("BasicAuth = (%q,%q,%v), want alice/s3cr3t/true", u, p, ok)
	}
}

func TestWithAPIKey(t *testing.T) {
	t.Parallel()

	var captured *http.Request
	c := dexpace.New(
		dexpace.WithTransport(captureTransport(&captured)),
		dexpace.WithAPIKey("X-API-Key", "secret-key"),
	)
	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if got := captured.Header.Get("X-API-Key"); got != "secret-key" {
		t.Fatalf("X-API-Key = %q, want secret-key", got)
	}
}

func TestWithDigestAuth(t *testing.T) {
	t.Parallel()

	const (
		user  = "u"
		pass  = "pw"
		realm = "test"
		nonce = "abc123"
	)

	// sha256 helper for the server-side recomputation of the RFC 7616 response.
	sum := func(s string) string {
		h := sha256.Sum256([]byte(s))
		return hex.EncodeToString(h[:])
	}
	param := func(hdr, key string) string {
		for _, raw := range strings.Split(strings.TrimPrefix(hdr, "Digest "), ",") {
			k, v, ok := strings.Cut(strings.TrimSpace(raw), "=")
			if ok && strings.EqualFold(k, key) {
				return strings.Trim(v, `"`)
			}
		}
		return ""
	}

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authz := r.Header.Get("Authorization")
		if !strings.HasPrefix(authz, "Digest ") {
			w.Header().Set("WWW-Authenticate",
				fmt.Sprintf(`Digest realm=%q, qop="auth", nonce=%q, algorithm=SHA-256`, realm, nonce))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		ha1 := sum(user + ":" + realm + ":" + pass)
		ha2 := sum(r.Method + ":" + param(authz, "uri"))
		want := sum(strings.Join([]string{ha1, nonce, param(authz, "nc"), param(authz, "cnonce"), "auth", ha2}, ":"))
		if param(authz, "response") != want {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(srv.Close)

	c := dexpace.New(
		dexpace.WithTransport(srv.Client()),
		dexpace.WithDigestAuth(user, pass),
		dexpace.WithoutIdempotency(),
	)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/resource", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 after Digest challenge", resp.StatusCode)
	}
}

func TestAuthPrecedenceBearerBeatsBasic(t *testing.T) {
	t.Parallel()

	var captured *http.Request
	// WithBasicAuth listed BEFORE WithCredential; bearer must still win.
	c := dexpace.New(
		dexpace.WithTransport(captureTransport(&captured)),
		dexpace.WithBasicAuth("alice", "s3cr3t"),
		dexpace.WithCredential(auth.StaticToken("tok")),
	)
	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if got := captured.Header.Get("Authorization"); got != "Bearer tok" {
		t.Fatalf("Authorization = %q, want \"Bearer tok\" (bearer beats basic)", got)
	}
	if _, _, ok := captured.BasicAuth(); ok {
		t.Fatal("basic auth should not be applied when a bearer credential is set")
	}
}

type countingCred struct{ calls int }

func (c *countingCred) GetToken(context.Context, auth.TokenRequestOptions) (auth.AccessToken, error) {
	c.calls++
	return auth.AccessToken{Token: "t", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

func TestWithTokenCacheSharedAcrossClients(t *testing.T) {
	t.Parallel()

	cred := &countingCred{}
	cache := auth.NewInMemoryTokenCache()

	for range 2 {
		var captured *http.Request
		c := dexpace.New(
			dexpace.WithTransport(captureTransport(&captured)),
			dexpace.WithCredential(cred, "scope"),
			dexpace.WithTokenCache(cache),
		)
		req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
		resp, err := c.Do(req)
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		_ = resp.Body.Close()
	}

	if cred.calls != 1 {
		t.Fatalf("GetToken calls = %d, want 1 (token cache shared across clients)", cred.calls)
	}
}

func TestWithConfigAppliesHTTPTimeout(t *testing.T) {
	t.Setenv("DEXPACE_HTTP_TIMEOUT", "30ms")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	// No WithTransport: the default transport is built, so the config timeout applies.
	// Disable retries so the timeout error surfaces promptly.
	c := dexpace.New(
		dexpace.WithConfig(config.New()),
		dexpace.WithRetry(retry.Options{MaxRetries: -1}),
	)
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	resp, err := c.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("expected a timeout error from the 30ms DEXPACE_HTTP_TIMEOUT")
	}
}
