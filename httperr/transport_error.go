// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package httperr

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"github.com/dexpace/go-sdk/redact"
)

// TransportError reports that a request never produced a response — for example
// a DNS failure, a refused connection, a TLS error, or a network timeout. It
// wraps the underlying net/http cause; Unwrap preserves errors.Is/As to that
// cause (including *url.Error and net.Error).
//
// It is returned only when the typed error model is enabled (dexpace.WithErrors).
type TransportError struct {
	// Method is the request method that failed.
	Method string
	// URL is the redacted request URL.
	URL string
	// Err is the underlying cause.
	Err error
}

// Error implements error. When the underlying cause is a *url.Error (as produced
// by net/http), its message — which embeds the full, unredacted URL — is replaced
// by the inner cause, so a query secret in the URL never reaches the error string.
// The redacted URL is reported via the Method/URL fields instead.
func (e *TransportError) Error() string {
	cause := e.Err
	var ue *url.Error
	if errors.As(e.Err, &ue) {
		cause = ue.Err
	}
	if e.URL == "" {
		return fmt.Sprintf("transport error: %v", cause)
	}
	return fmt.Sprintf("transport error: %s %s: %v", e.Method, e.URL, cause)
}

// Unwrap returns the underlying cause so errors.Is/As reach through it.
func (e *TransportError) Unwrap() error { return e.Err }

// Timeout reports whether the underlying cause is a network timeout. It mirrors
// net.Error.Timeout(); it is false for non-net causes.
func (e *TransportError) Timeout() bool {
	var ne net.Error
	return errors.As(e.Err, &ne) && ne.Timeout()
}

// FromError maps a transport-level error to the typed model. It returns nil for
// a nil error and returns context cancellation/deadline errors unchanged (they
// are the caller's deadline, not a transport fault). Any other error is wrapped
// in a [TransportError] carrying req's method and redacted URL.
func FromError(err error, req *http.Request) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	te := &TransportError{Err: err}
	if req != nil {
		te.Method = req.Method
		if req.URL != nil {
			te.URL = redact.Default.URL(req.URL)
		}
	}
	return te
}
