// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package redact_test

import (
	"net/url"
	"testing"

	"github.com/dexpace/go-sdk/redact"
)

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return u
}

func TestDefaultRedactsUserinfoAndAllQueryValues(t *testing.T) {
	t.Parallel()

	u := mustURL(t, "https://user:pass@api.example.test/things?api_key=secret&page=2")
	got := redact.Default.URL(u)

	want := "https://api.example.test/things?api_key=REDACTED&page=REDACTED"
	if got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}
}

func TestAllowlistKeepsListedValues(t *testing.T) {
	t.Parallel()

	r := redact.New("page")
	u := mustURL(t, "https://api.example.test/things?api_key=secret&page=2")
	got := r.URL(u)

	want := "https://api.example.test/things?api_key=REDACTED&page=2"
	if got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}
}

func TestNilURL(t *testing.T) {
	t.Parallel()

	if got := redact.Default.URL(nil); got != "" {
		t.Fatalf("URL(nil) = %q, want empty", got)
	}
}

func TestPreservesPathAndFragmentNoQuery(t *testing.T) {
	t.Parallel()

	u := mustURL(t, "https://api.example.test/a/b#frag")
	if got := redact.Default.URL(u); got != "https://api.example.test/a/b#frag" {
		t.Fatalf("URL = %q, want path and fragment preserved", got)
	}
}
