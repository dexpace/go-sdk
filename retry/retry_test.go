// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package retry_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dexpace/go-sdk/idempotency"
	"github.com/dexpace/go-sdk/pipeline"
	"github.com/dexpace/go-sdk/retry"
)

type transporterFunc func(*http.Request) (*http.Response, error)

func (f transporterFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

func statusResponse(req *http.Request, code int) *http.Response {
	return &http.Response{
		StatusCode: code,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("")),
		Request:    req,
	}
}

// fastOptions keeps backoff negligible so tests stay quick.
func fastOptions(maxRetries int) retry.Options {
	return retry.Options{
		MaxRetries: maxRetries,
		BaseDelay:  time.Microsecond,
		MaxDelay:   time.Millisecond,
	}
}

func TestRetriesThenSucceeds(t *testing.T) {
	t.Parallel()

	calls := 0
	transport := transporterFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls < 3 {
			return statusResponse(req, http.StatusServiceUnavailable), nil
		}
		return statusResponse(req, http.StatusOK), nil
	})

	pl := pipeline.New(transport, retry.NewPolicy(fastOptions(5)))
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if calls != 3 {
		t.Fatalf("transport calls = %d, want 3", calls)
	}
}

func TestStopsAtMaxRetries(t *testing.T) {
	t.Parallel()

	calls := 0
	transport := transporterFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return statusResponse(req, http.StatusBadGateway), nil
	})

	pl := pipeline.New(transport, retry.NewPolicy(fastOptions(2)))
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	// 1 initial attempt + 2 retries = 3 calls.
	if calls != 3 {
		t.Fatalf("transport calls = %d, want 3", calls)
	}
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}
}

func TestDoesNotRetryNonRetryableStatus(t *testing.T) {
	t.Parallel()

	calls := 0
	transport := transporterFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return statusResponse(req, http.StatusBadRequest), nil
	})

	pl := pipeline.New(transport, retry.NewPolicy(fastOptions(5)))
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if calls != 1 {
		t.Fatalf("transport calls = %d, want 1", calls)
	}
}

func TestRetriesTransportError(t *testing.T) {
	t.Parallel()

	calls := 0
	wantErr := errors.New("dial failed")
	transport := transporterFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, wantErr
	})

	pl := pipeline.New(transport, retry.NewPolicy(fastOptions(2)))
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	_, err := pl.Do(req)
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if calls != 3 {
		t.Fatalf("transport calls = %d, want 3", calls)
	}
}

func TestDoesNotRetryContextCancellation(t *testing.T) {
	t.Parallel()

	calls := 0
	transport := transporterFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, context.Canceled
	})

	pl := pipeline.New(transport, retry.NewPolicy(fastOptions(5)))
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	_, err := pl.Do(req)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Fatalf("transport calls = %d, want 1", calls)
	}
}

// countingTransport always fails with a transport error and records attempts.
func countingTransport(calls *int) transporterFunc {
	return func(*http.Request) (*http.Response, error) {
		*calls++
		return nil, errors.New("dial tcp: connection refused")
	}
}

func TestRetriesGetOnTransportError(t *testing.T) {
	t.Parallel()

	var calls int
	pl := pipeline.New(countingTransport(&calls),
		retry.NewPolicy(retry.Options{MaxRetries: 2, BaseDelay: 1, MaxDelay: 1}))
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	_, _ = pl.Do(req)

	if calls != 3 { // initial + 2 retries
		t.Fatalf("GET attempts = %d, want 3", calls)
	}
}

func TestDoesNotRetryUnkeyedPost(t *testing.T) {
	t.Parallel()

	var calls int
	pl := pipeline.New(countingTransport(&calls),
		retry.NewPolicy(retry.Options{MaxRetries: 2, BaseDelay: 1, MaxDelay: 1}))
	req, _ := http.NewRequest(http.MethodPost, "https://example.test/", nil)
	_, _ = pl.Do(req)

	if calls != 1 { // no retries for a non-idempotent POST
		t.Fatalf("unkeyed POST attempts = %d, want 1", calls)
	}
}

func TestRetriesKeyedPost(t *testing.T) {
	t.Parallel()

	var calls int
	pl := pipeline.New(countingTransport(&calls),
		retry.NewPolicy(retry.Options{MaxRetries: 2, BaseDelay: 1, MaxDelay: 1}),
		idempotency.NewPolicy(idempotency.Options{}))
	req, _ := http.NewRequest(http.MethodPost, "https://example.test/", nil)
	_, _ = pl.Do(req)

	if calls != 3 { // POST is now retry-safe because it carries an idempotency key
		t.Fatalf("keyed POST attempts = %d, want 3", calls)
	}
}

func TestNegativeMaxRetriesDisablesRetry(t *testing.T) {
	t.Parallel()

	calls := 0
	transport := transporterFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return statusResponse(req, http.StatusServiceUnavailable), nil
	})

	pl := pipeline.New(transport, retry.NewPolicy(retry.Options{MaxRetries: -1}))
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if calls != 1 {
		t.Fatalf("transport calls = %d, want 1", calls)
	}
}
