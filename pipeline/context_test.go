// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package pipeline_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/dexpace/go-sdk/pipeline"
)

func TestSetContextPropagatesDownstream(t *testing.T) {
	t.Parallel()

	type ctxKey struct{}
	setter := pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		req.SetContext(context.WithValue(req.Raw().Context(), ctxKey{}, "v"))
		return req.Next()
	})
	var got any
	reader := pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		got = req.Raw().Context().Value(ctxKey{})
		return req.Next()
	})

	pl := pipeline.New(transporterFunc(okResponse), setter, reader)
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if got != "v" {
		t.Fatalf("downstream context value = %v, want \"v\"", got)
	}
}

func TestSetContextIgnoresNil(t *testing.T) {
	t.Parallel()

	p := pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		req.SetContext(nil) // must not panic
		return req.Next()
	})
	pl := pipeline.New(transporterFunc(okResponse), p)
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
}
