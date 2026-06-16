// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package conditions

import (
	"fmt"
	"net/http"

	"github.com/dexpace/go-sdk/header"
)

// Range is an HTTP byte range for the Range header (RFC 9110 §14.2).
type Range struct {
	start  int64
	end    int64
	hasEnd bool
}

// Bytes returns the inclusive byte range [start, end].
func Bytes(start, end int64) Range {
	return Range{start: start, end: end, hasEnd: true}
}

// BytesFrom returns the open-ended byte range [start, end-of-resource).
func BytesFrom(start int64) Range {
	return Range{start: start}
}

// String returns the Range header value, "bytes=start-end" or "bytes=start-".
func (r Range) String() string {
	if r.hasEnd {
		return fmt.Sprintf("bytes=%d-%d", r.start, r.end)
	}
	return fmt.Sprintf("bytes=%d-", r.start)
}

// Apply sets the Range header on req.
func (r Range) Apply(req *http.Request) {
	req.Header.Set(header.Range, r.String())
}
