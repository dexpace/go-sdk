// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package header provides canonical HTTP header-name constants used across the
// SDK.
//
// Each value is in the canonical form produced by net/http.CanonicalHeaderKey
// (the form http.Header stores keys in), so the constants can be used both with
// http.Header.Get/Set and as direct map keys. A few canonical spellings look
// unusual — "Etag", "Www-Authenticate", "X-Request-Id" — and are noted inline.
package header

// Request and response header names in canonical form.
const (
	Accept          = "Accept"
	AcceptEncoding  = "Accept-Encoding"
	Authorization   = "Authorization"
	CacheControl    = "Cache-Control"
	ContentEncoding = "Content-Encoding"
	ContentLength   = "Content-Length"
	ContentType     = "Content-Type"
	Date            = "Date"
	ETag            = "Etag" // canonical form of "ETag"
	IfMatch         = "If-Match"
	IfNoneMatch     = "If-None-Match"
	Location        = "Location"
	RetryAfter      = "Retry-After"
	UserAgent       = "User-Agent"
	WWWAuthenticate = "Www-Authenticate" // canonical form of "WWW-Authenticate"
	XRequestID      = "X-Request-Id"     // canonical form of "X-Request-ID"
)
