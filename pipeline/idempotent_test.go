// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package pipeline_test

import (
	"net/http"
	"testing"

	"github.com/dexpace/go-sdk/pipeline"
)

func TestIdempotentMarker(t *testing.T) {
	t.Parallel()

	var beforeMark, afterMark bool
	marker := pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		beforeMark = pipeline.IsIdempotent(req)
		pipeline.MarkIdempotent(req)
		return req.Next()
	})
	reader := pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		afterMark = pipeline.IsIdempotent(req)
		return req.Next()
	})

	pl := pipeline.New(transporterFunc(okResponse), marker, reader)
	req, _ := http.NewRequest(http.MethodPost, "https://example.test/", nil)
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if beforeMark {
		t.Fatal("IsIdempotent true before MarkIdempotent")
	}
	if !afterMark {
		t.Fatal("IsIdempotent false after MarkIdempotent")
	}
}
