// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package header_test

import (
	"net/http"
	"testing"

	"github.com/dexpace/go-sdk/header"
)

func TestNewHeaderConstantsAreCanonical(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		header.Range:             "Range",
		header.IfModifiedSince:   "If-Modified-Since",
		header.IfUnmodifiedSince: "If-Unmodified-Since",
	}
	for got, want := range cases {
		if got != want {
			t.Fatalf("constant = %q, want %q", got, want)
		}
		if canon := http.CanonicalHeaderKey(want); canon != got {
			t.Fatalf("constant %q is not canonical (canonical is %q)", got, canon)
		}
	}
}
