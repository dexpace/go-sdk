// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package pipeline

// idempotentKey is the unexported request-value key under which an idempotency
// marker is stored. Using a private zero-size type avoids collisions with other
// packages' request values.
type idempotentKey struct{}

// MarkIdempotent records that req is safe to retry even if its HTTP method is not
// inherently idempotent — for example a POST carrying an Idempotency-Key. The
// retry policy consults this via [IsIdempotent].
func MarkIdempotent(req *Request) { req.SetValue(idempotentKey{}, true) }

// IsIdempotent reports whether [MarkIdempotent] was called for req.
func IsIdempotent(req *Request) bool {
	v, ok := req.Value(idempotentKey{}).(bool)
	return ok && v
}
