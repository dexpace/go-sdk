// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package config_test

import (
	"testing"
	"time"

	"github.com/dexpace/go-sdk/config"
)

func TestGetStringDefaultWhenUnset(t *testing.T) {
	t.Parallel()

	c := config.New()
	if got := c.GetString("DEXPACE_NOT_SET_XYZ", "fallback"); got != "fallback" {
		t.Fatalf("GetString = %q, want fallback", got)
	}
}

func TestOverrideBeatsDefault(t *testing.T) {
	t.Parallel()

	c := config.New(config.WithOverride("K", "v"))
	if got := c.GetString("K", "def"); got != "v" {
		t.Fatalf("GetString = %q, want v", got)
	}
}

func TestGetIntParsesAndFallsBack(t *testing.T) {
	t.Parallel()

	c := config.New(config.WithOverride("N", "7"), config.WithOverride("BAD", "x"))
	if got := c.GetInt("N", 1); got != 7 {
		t.Fatalf("GetInt(N) = %d, want 7", got)
	}
	if got := c.GetInt("BAD", 1); got != 1 {
		t.Fatalf("GetInt(BAD) = %d, want 1 (parse fallback)", got)
	}
	if got := c.GetInt("MISSING", 3); got != 3 {
		t.Fatalf("GetInt(MISSING) = %d, want 3", got)
	}
}

func TestGetBoolParsesAndFallsBack(t *testing.T) {
	t.Parallel()

	c := config.New(
		config.WithOverride("T", "true"),
		config.WithOverride("ONE", "1"),
		config.WithOverride("BAD", "nope"),
	)
	if !c.GetBool("T", false) {
		t.Fatal("GetBool(T) = false, want true")
	}
	if !c.GetBool("ONE", false) {
		t.Fatal("GetBool(ONE) = false, want true")
	}
	if c.GetBool("BAD", false) {
		t.Fatal("GetBool(BAD) = true, want false (parse fallback)")
	}
}

func TestGetDurationParsesAndFallsBack(t *testing.T) {
	t.Parallel()

	c := config.New(config.WithOverride("D", "2m30s"), config.WithOverride("BAD", "xyz"))
	if got := c.GetDuration("D", time.Second); got != 150*time.Second {
		t.Fatalf("GetDuration(D) = %v, want 2m30s", got)
	}
	if got := c.GetDuration("BAD", time.Second); got != time.Second {
		t.Fatalf("GetDuration(BAD) = %v, want 1s (parse fallback)", got)
	}
}

func TestWithOverridesCopiesMap(t *testing.T) {
	t.Parallel()

	m := map[string]string{"K": "v"}
	c := config.New(config.WithOverrides(m))
	m["K"] = "mutated"
	if got := c.GetString("K", "def"); got != "v" {
		t.Fatalf("GetString = %q, want v (map should be copied)", got)
	}
}

func TestEnvUsedWhenNoOverride(t *testing.T) {
	t.Setenv("DEXPACE_TEST_ENV_KEY", "from-env")
	c := config.New()
	if got := c.GetString("DEXPACE_TEST_ENV_KEY", "def"); got != "from-env" {
		t.Fatalf("GetString = %q, want from-env", got)
	}
}

func TestOverrideBeatsEnv(t *testing.T) {
	t.Setenv("DEXPACE_TEST_ENV_KEY2", "from-env")
	c := config.New(config.WithOverride("DEXPACE_TEST_ENV_KEY2", "from-override"))
	if got := c.GetString("DEXPACE_TEST_ENV_KEY2", "def"); got != "from-override" {
		t.Fatalf("GetString = %q, want from-override", got)
	}
}

func TestKeysAreNamespaced(t *testing.T) {
	t.Parallel()

	for _, k := range []string{
		config.EnvMaxRetries, config.EnvRetryBaseDelay,
		config.EnvHTTPTimeout, config.EnvUserAgent,
	} {
		if len(k) < len("DEXPACE_") || k[:len("DEXPACE_")] != "DEXPACE_" {
			t.Fatalf("key %q is not DEXPACE_-prefixed", k)
		}
	}
}
