// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package dexpace_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	dexpace "github.com/dexpace/go-sdk"
	"github.com/dexpace/go-sdk/httperr"
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
			Status:     http.StatusText(code),
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
