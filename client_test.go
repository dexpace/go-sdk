// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package dexpace_test

import (
	"net/http"
	"testing"

	dexpace "github.com/dexpace/go-sdk"
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
