// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package conditions_test

import (
	"testing"

	"github.com/dexpace/go-sdk/conditions"
)

func TestETagString(t *testing.T) {
	t.Parallel()

	if got := conditions.NewETag("abc").String(); got != `"abc"` {
		t.Fatalf("strong ETag = %q, want \"abc\"", got)
	}
	if got := conditions.NewWeakETag("abc").String(); got != `W/"abc"` {
		t.Fatalf("weak ETag = %q, want W/\"abc\"", got)
	}
}

func TestETagParse(t *testing.T) {
	t.Parallel()

	strong, err := conditions.Parse(`"abc"`)
	if err != nil {
		t.Fatalf("Parse strong: %v", err)
	}
	if strong.Tag() != "abc" || strong.Weak() {
		t.Fatalf("strong = %+v, want tag=abc weak=false", strong)
	}

	weak, err := conditions.Parse(`W/"abc"`)
	if err != nil {
		t.Fatalf("Parse weak: %v", err)
	}
	if weak.Tag() != "abc" || !weak.Weak() {
		t.Fatalf("weak = %+v, want tag=abc weak=true", weak)
	}

	for _, bad := range []string{"", "abc", `"abc`, `abc"`, "W/abc"} {
		if _, err := conditions.Parse(bad); err == nil {
			t.Fatalf("Parse(%q) should fail", bad)
		}
	}
}

func TestETagRoundTrip(t *testing.T) {
	t.Parallel()

	for _, e := range []conditions.ETag{conditions.NewETag("x"), conditions.NewWeakETag("y")} {
		got, err := conditions.Parse(e.String())
		if err != nil {
			t.Fatalf("Parse(%q): %v", e.String(), err)
		}
		if got != e {
			t.Fatalf("round-trip = %+v, want %+v", got, e)
		}
	}
}
