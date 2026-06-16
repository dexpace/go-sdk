// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package config

import (
	"os"
	"strconv"
	"time"
)

// Config resolves string-keyed values from explicit overrides, then the process
// environment, then a caller-supplied default. It is safe for concurrent use; its
// methods tolerate a nil receiver (environment-only lookup).
type Config struct {
	overrides map[string]string
}

// Option configures a [Config].
type Option func(*Config)

// New builds a Config from the given options. With no options it resolves from
// the environment only.
func New(opts ...Option) *Config {
	c := &Config{overrides: make(map[string]string)}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// WithOverride sets a single override that takes precedence over the environment.
func WithOverride(key, value string) Option {
	return func(c *Config) { c.overrides[key] = value }
}

// WithOverrides copies the entries of m into the Config's overrides. Later
// mutation of m does not affect the Config.
func WithOverrides(m map[string]string) Option {
	return func(c *Config) {
		for k, v := range m {
			c.overrides[k] = v
		}
	}
}

// Lookup returns the value for key — an override first, then the environment —
// and whether it was found.
func (c *Config) Lookup(key string) (string, bool) {
	if c != nil {
		if v, ok := c.overrides[key]; ok {
			return v, true
		}
	}
	return os.LookupEnv(key)
}

// GetString returns the value for key, or def when unset.
func (c *Config) GetString(key, def string) string {
	if v, ok := c.Lookup(key); ok {
		return v
	}
	return def
}

// GetInt returns the value parsed as a base-10 int, or def when unset or
// unparseable.
func (c *Config) GetInt(key string, def int) int {
	v, ok := c.Lookup(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// GetBool returns the value parsed with strconv.ParseBool, or def when unset or
// unparseable.
func (c *Config) GetBool(key string, def bool) bool {
	v, ok := c.Lookup(key)
	if !ok {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

// GetDuration returns the value parsed with time.ParseDuration (Go shorthand such
// as "5s" or "2m30s"), or def when unset or unparseable.
func (c *Config) GetDuration(key string, def time.Duration) time.Duration {
	v, ok := c.Lookup(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
