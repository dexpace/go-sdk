// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package conditions_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/dexpace/go-sdk/conditions"
)

func TestConditionsApplyIfNoneMatch(t *testing.T) {
	t.Parallel()

	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
	conditions.Conditions{
		IfNoneMatch: []conditions.ETag{conditions.NewETag("a"), conditions.NewWeakETag("b")},
	}.Apply(req)

	if got := req.Header.Get("If-None-Match"); got != `"a", W/"b"` {
		t.Fatalf("If-None-Match = %q, want \"a\", W/\"b\"", got)
	}
	if req.Header.Get("If-Match") != "" {
		t.Fatal("If-Match should be unset")
	}
}

func TestConditionsApplyModifiedSince(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
	conditions.Conditions{IfModifiedSince: ts}.Apply(req)

	if got := req.Header.Get("If-Modified-Since"); got != ts.Format(http.TimeFormat) {
		t.Fatalf("If-Modified-Since = %q, want %q", got, ts.Format(http.TimeFormat))
	}
}

func TestConditionsApplyEmptyIsNoOp(t *testing.T) {
	t.Parallel()

	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
	conditions.Conditions{}.Apply(req)

	for _, h := range []string{"If-Match", "If-None-Match", "If-Modified-Since", "If-Unmodified-Since"} {
		if req.Header.Get(h) != "" {
			t.Fatalf("%s should be unset for empty Conditions", h)
		}
	}
}
