// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package auth_test

import (
	"sync"
	"testing"

	"github.com/dexpace/go-sdk/auth"
)

func TestInMemoryTokenCacheGetSet(t *testing.T) {
	t.Parallel()

	c := auth.NewInMemoryTokenCache()
	if _, ok := c.Get("k"); ok {
		t.Fatal("missing key should report not found")
	}
	c.Set("k", auth.AccessToken{Token: "t"})
	got, ok := c.Get("k")
	if !ok || got.Token != "t" {
		t.Fatalf("Get = (%v, %v), want token t / true", got, ok)
	}
}

func TestInMemoryTokenCacheConcurrent(t *testing.T) {
	t.Parallel()

	c := auth.NewInMemoryTokenCache()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Set("k", auth.AccessToken{Token: "t"})
			_, _ = c.Get("k")
		}()
	}
	wg.Wait()
}

func TestInMemoryTokenCacheKeysAreIsolated(t *testing.T) {
	t.Parallel()

	c := auth.NewInMemoryTokenCache()
	c.Set("a", auth.AccessToken{Token: "ta"})
	c.Set("a b", auth.AccessToken{Token: "tab"})

	if got, ok := c.Get("a"); !ok || got.Token != "ta" {
		t.Fatalf("Get(\"a\") = (%v, %v), want ta/true", got, ok)
	}
	if got, ok := c.Get("a b"); !ok || got.Token != "tab" {
		t.Fatalf("Get(\"a b\") = (%v, %v), want tab/true", got, ok)
	}
	if _, ok := c.Get("b"); ok {
		t.Fatal("Get(\"b\") should be a miss (distinct key)")
	}
}
