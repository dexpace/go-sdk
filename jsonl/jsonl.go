// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package jsonl

import (
	"encoding/json"
	"errors"
	"io"
	"iter"
)

// Decode reads a stream of JSON values from r and yields each decoded into a T.
// Values may be separated by any JSON whitespace (newlines for NDJSON / JSON
// Lines, or none). Iteration ends at end of stream; a decode error is delivered
// as the second iteration value, after which iteration stops. The iterator is
// single-pass.
func Decode[T any](r io.Reader) iter.Seq2[T, error] {
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
