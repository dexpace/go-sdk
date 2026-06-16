// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package httperr_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/dexpace/go-sdk/httperr"
)

func newReq() *http.Request {
	return &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Scheme: "https", Host: "api.example.test", Path: "/things"},
	}
}

// timeoutErr is a net.Error reporting a timeout.
type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return false }

func TestFromErrorNil(t *testing.T) {
	t.Parallel()

	if got := httperr.FromError(nil, newReq()); got != nil {
		t.Fatalf("FromError(nil) = %v, want nil", got)
	}
}

func TestFromErrorPassesThroughContextErrors(t *testing.T) {
	t.Parallel()

	wrapped := &url.Error{Op: "Get", URL: "https://x", Err: context.Canceled}
	got := httperr.FromError(wrapped, newReq())

	var te *httperr.TransportError
	if errors.As(got, &te) {
		t.Fatal("context error must NOT be wrapped as *TransportError")
	}
	if !errors.Is(got, context.Canceled) {
		t.Fatalf("FromError lost context.Canceled: %v", got)
	}
}

func TestFromErrorWrapsTransportFailure(t *testing.T) {
	t.Parallel()

	cause := errors.New("dial tcp: connection refused")
	got := httperr.FromError(cause, newReq())

	var te *httperr.TransportError
	if !errors.As(got, &te) {
		t.Fatalf("FromError = %T, want *TransportError", got)
	}
	if te.Method != http.MethodGet {
		t.Fatalf("Method = %q, want GET", te.Method)
	}
	if te.URL != "https://api.example.test/things" {
		t.Fatalf("URL = %q, want the redacted request URL", te.URL)
	}
	if !errors.Is(got, cause) {
		t.Fatal("Unwrap must reach the underlying cause")
	}
}

func TestTransportErrorDoesNotLeakURLInMessage(t *testing.T) {
	t.Parallel()

	// net/http surfaces a *url.Error whose Error() embeds the full raw URL,
	// including query secrets. TransportError.Error() must not reproduce it.
	cause := &url.Error{
		Op:  "Get",
		URL: "https://api.example.test/things?token=SECRET",
		Err: errors.New("dial tcp: connection refused"),
	}
	req := &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Scheme: "https", Host: "api.example.test", Path: "/things", RawQuery: "token=SECRET"},
	}
	te := httperr.FromError(cause, req)

	msg := te.Error()
	if strings.Contains(msg, "SECRET") {
		t.Fatalf("Error() leaked the query secret: %q", msg)
	}
	if !strings.Contains(msg, "connection refused") {
		t.Fatalf("Error() should include the underlying cause: %q", msg)
	}
	// Unwrap must remain lossless: the original cause is still reachable.
	if !errors.Is(te, cause) {
		t.Fatal("Unwrap must still reach the original *url.Error cause")
	}
}

func TestTransportErrorTimeout(t *testing.T) {
	t.Parallel()

	timeout := httperr.FromError(timeoutErr{}, newReq())
	var te *httperr.TransportError
	if !errors.As(timeout, &te) || !te.Timeout() {
		t.Fatal("Timeout() should be true for a net.Error timeout cause")
	}

	plain := httperr.FromError(errors.New("nope"), newReq())
	var te2 *httperr.TransportError
	if !errors.As(plain, &te2) || te2.Timeout() {
		t.Fatal("Timeout() should be false for a non-net cause")
	}
}
