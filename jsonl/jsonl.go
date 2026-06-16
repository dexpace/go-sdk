// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package jsonl

import (
	"encoding/json"
	"errors"
	"io"
	"iter"
)

// Option configures [Decode].
type Option func(*config)

type config struct {
	maxBytes int64
}

// WithMaxBytes bounds the total number of bytes Decode reads from the stream, so
// an untrusted source cannot force unbounded memory use. A value <= 0 (the
// default) leaves the stream unbounded. When the limit is reached mid-value, the
// truncated value surfaces a decode error.
func WithMaxBytes(n int64) Option {
	return func(c *config) { c.maxBytes = n }
}

// Decode reads a stream of JSON values from r and yields each decoded into a T.
// Values may be separated by any JSON whitespace (newlines for NDJSON / JSON
// Lines, or none). Iteration ends at end of stream; a decode error is delivered
// as the second iteration value, after which iteration stops. The iterator is
// single-pass.
//
// Decode does not bound the size of an individual JSON value by default; for an
// untrusted stream, pass [WithMaxBytes] (or wrap r in an io.LimitReader) so a
// hostile value cannot exhaust memory.
func Decode[T any](r io.Reader, opts ...Option) iter.Seq2[T, error] {
	var cfg config
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.maxBytes > 0 {
		r = io.LimitReader(r, cfg.maxBytes)
	}
	return func(yield func(T, error) bool) {
		dec := json.NewDecoder(r)
		for {
			var v T
			err := dec.Decode(&v)
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				var zero T
				yield(zero, err)
				return
			}
			if !yield(v, nil) {
				return
			}
		}
	}
}
