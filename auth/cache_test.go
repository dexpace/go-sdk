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
