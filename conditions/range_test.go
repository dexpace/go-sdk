// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package conditions_test

import (
	"net/http"
	"testing"

	"github.com/dexpace/go-sdk/conditions"
)

func TestRangeString(t *testing.T) {
	t.Parallel()

	if got := conditions.Bytes(0, 99).String(); got != "bytes=0-99" {
		t.Fatalf("Bytes(0,99) = %q, want bytes=0-99", got)
	}
	if got := conditions.BytesFrom(100).String(); got != "bytes=100-" {
		t.Fatalf("BytesFrom(100) = %q, want bytes=100-", got)
	}
}

func TestRangeApply(t *testing.T) {
	t.Parallel()

	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
	conditions.Bytes(0, 1023).Apply(req)
	if got := req.Header.Get("Range"); got != "bytes=0-1023" {
		t.Fatalf("Range header = %q, want bytes=0-1023", got)
	}
}
