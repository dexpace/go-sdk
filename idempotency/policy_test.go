// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package idempotency

import (
	"regexp"
	"testing"
)

var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewUUIDv4Format(t *testing.T) {
	t.Parallel()

	for i := 0; i < 100; i++ {
		got, err := newUUIDv4()
		if err != nil {
			t.Fatalf("newUUIDv4: %v", err)
		}
		if !uuidV4.MatchString(got) {
			t.Fatalf("newUUIDv4 = %q, not a canonical UUIDv4", got)
		}
	}
}

func TestNewUUIDv4Unique(t *testing.T) {
	t.Parallel()

	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		got, err := newUUIDv4()
		if err != nil {
			t.Fatalf("newUUIDv4: %v", err)
		}
		if _, dup := seen[got]; dup {
			t.Fatalf("duplicate UUID %q", got)
		}
		seen[got] = struct{}{}
	}
}
