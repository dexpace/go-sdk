// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package httperr_test

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/dexpace/go-sdk/httperr"
)

func newResponse(code int, body string) *http.Response {
	return &http.Response{
		StatusCode: code,
		Status:     http.StatusText(code),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request: &http.Request{
			Method: http.MethodGet,
			URL:    &url.URL{Scheme: "https", Host: "api.example.test", Path: "/things"},
		},
	}
}

func TestFromResponseReturnsNilForSuccess(t *testing.T) {
	t.Parallel()

	if rerr := httperr.FromResponse(newResponse(http.StatusOK, "ok")); rerr != nil {
		t.Fatalf("expected nil for 200, got %v", rerr)
	}
	if rerr := httperr.FromResponse(nil); rerr != nil {
		t.Fatalf("expected nil for nil response, got %v", rerr)
	}
}

func TestFromResponseCapturesError(t *testing.T) {
	t.Parallel()

	resp := newResponse(http.StatusNotFound, "missing")
	rerr := httperr.FromResponse(resp)
	if rerr == nil {
		t.Fatal("expected a ResponseError for 404")
	}
	if rerr.StatusCode != http.StatusNotFound {
		t.Fatalf("StatusCode = %d, want 404", rerr.StatusCode)
	}
	if string(rerr.Body()) != "missing" {
		t.Fatalf("Body = %q, want %q", rerr.Body(), "missing")
	}
	if !strings.Contains(rerr.Error(), "https://api.example.test/things") {
		t.Fatalf("Error() = %q, want it to contain the URL", rerr.Error())
	}

	// The body must still be readable by the caller after capture.
	rest, _ := io.ReadAll(resp.Body)
	if string(rest) != "missing" {
		t.Fatalf("rewound body = %q, want %q", rest, "missing")
	}
}

func TestErrorsAsExtractsResponseError(t *testing.T) {
	t.Parallel()

	var err error = httperr.FromResponse(newResponse(http.StatusBadGateway, ""))
	var rerr *httperr.ResponseError
	if !errors.As(err, &rerr) {
		t.Fatal("errors.As should extract *ResponseError")
	}
	if rerr.StatusCode != http.StatusBadGateway {
		t.Fatalf("StatusCode = %d, want 502", rerr.StatusCode)
	}
}

func TestResponseErrorRedactsQuery(t *testing.T) {
	t.Parallel()

	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(strings.NewReader("bad")),
		Request: &http.Request{
			Method: http.MethodGet,
			URL:    &url.URL{Scheme: "https", Host: "api.example.test", Path: "/things", RawQuery: "api_key=secret"},
		},
	}
	rerr := httperr.FromResponse(resp)
	if rerr == nil {
		t.Fatal("expected a ResponseError")
	}
	if strings.Contains(rerr.URL, "secret") {
		t.Fatalf("URL %q leaked the query secret", rerr.URL)
	}
	if !strings.Contains(rerr.URL, "api_key=REDACTED") {
		t.Fatalf("URL %q should show api_key=REDACTED", rerr.URL)
	}
}
