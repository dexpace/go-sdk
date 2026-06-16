// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package httperr defines the error types the SDK returns for HTTP-level
// failures, principally [ResponseError] for non-success status codes.
package httperr

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/dexpace/go-sdk/redact"
	"github.com/dexpace/go-sdk/serde"
)

// maxErrorBodyBytes caps how much of an error response body is buffered for
// diagnostics, so a large error page cannot exhaust memory.
const maxErrorBodyBytes = 8 << 10 // 8 KiB

// ResponseError represents an HTTP response carrying a non-success (>= 400)
// status code. The raw response is retained, and its body is drained into a
// buffer and rewound so callers can still read it. Extract it with errors.As:
//
//	var rerr *httperr.ResponseError
//	if errors.As(err, &rerr) && rerr.StatusCode == http.StatusNotFound {
//		// handle 404
//	}
type ResponseError struct {
	// StatusCode is the HTTP status code (e.g. 404).
	StatusCode int
	// Status is the HTTP status line (e.g. "404 Not Found").
	Status string
	// Method is the request method that produced the error.
	Method string
	// URL is the redacted request URL.
	URL string
	// RawResponse is the original response. Its Body has been replaced with an
	// in-memory reader over the buffered (possibly truncated) body.
	RawResponse *http.Response

	body []byte
}

// Error implements error.
func (e *ResponseError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "http response error: %s %s: %s", e.Method, e.URL, e.statusText())
	if len(e.body) > 0 {
		b.WriteByte('\n')
		b.Write(e.body)
	}
	return b.String()
}

func (e *ResponseError) statusText() string {
	if e.Status != "" {
		return e.Status
	}
	return fmt.Sprintf("%d %s", e.StatusCode, http.StatusText(e.StatusCode))
}

// Body returns the buffered response body, truncated to an internal cap.
func (e *ResponseError) Body() []byte { return e.body }

// DecodeInto unmarshals the buffered error-response body into v as JSON, using
// the SDK's default serde. It returns an error when the body is empty or cannot
// be decoded. v must be a non-nil pointer. The body is the one captured by
// FromResponse, truncated to the internal cap.
func (e *ResponseError) DecodeInto(v any) error {
	if len(e.body) == 0 {
		return errors.New("httperr: response has no body to decode")
	}
	return serde.JSON.Unmarshal(e.body, v)
}

// FromResponse builds a [ResponseError] from resp, buffering and rewinding its
// body so the caller may still read resp.Body. It returns nil when resp is nil
// or carries a success (< 400) status code, so it composes cleanly:
//
//	resp, err := client.Do(req)
//	if err != nil {
//		return err
//	}
//	if rerr := httperr.FromResponse(resp); rerr != nil {
//		return rerr
//	}
func FromResponse(resp *http.Response) *ResponseError {
	if resp == nil || resp.StatusCode < 400 {
		return nil
	}
	rerr := &ResponseError{
		StatusCode:  resp.StatusCode,
		Status:      resp.Status,
		RawResponse: resp,
		body:        drain(resp),
	}
	if resp.Request != nil {
		rerr.Method = resp.Request.Method
		if resp.Request.URL != nil {
			rerr.URL = redact.Default.URL(resp.Request.URL)
		}
	}
	return rerr
}

// drain reads up to maxErrorBodyBytes from the body, closes it, and replaces it
// with an in-memory reader over the captured bytes so callers can re-read them.
func drain(resp *http.Response) []byte {
	if resp.Body == nil {
		return nil
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(data))
	return data
}
