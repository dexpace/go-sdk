// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package dexpace_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"

	dexpace "github.com/dexpace/go-sdk"
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
