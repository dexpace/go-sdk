// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package conditions

import (
	"net/http"
	"strings"
	"time"

	"github.com/dexpace/go-sdk/header"
)

// Conditions carries conditional-request headers (RFC 9110 §13). Empty ETag
// slices and zero times are left unset.
type Conditions struct {
	IfMatch           []ETag
	IfNoneMatch       []ETag
	IfModifiedSince   time.Time
	IfUnmodifiedSince time.Time
}

// Apply sets the configured conditional headers on req. Each ETag list is
// comma-joined; times are formatted as HTTP-dates in UTC. Unset fields leave the
// corresponding header untouched; set fields overwrite any existing value.
func (c Conditions) Apply(req *http.Request) {
	if v := joinETags(c.IfMatch); v != "" {
		req.Header.Set(header.IfMatch, v)
	}
	if v := joinETags(c.IfNoneMatch); v != "" {
		req.Header.Set(header.IfNoneMatch, v)
	}
	if !c.IfModifiedSince.IsZero() {
		req.Header.Set(header.IfModifiedSince, c.IfModifiedSince.UTC().Format(http.TimeFormat))
	}
	if !c.IfUnmodifiedSince.IsZero() {
		req.Header.Set(header.IfUnmodifiedSince, c.IfUnmodifiedSince.UTC().Format(http.TimeFormat))
	}
}

func joinETags(tags []ETag) string {
	if len(tags) == 0 {
		return ""
	}
	parts := make([]string, len(tags))
	for i, t := range tags {
		parts[i] = t.String()
	}
	return strings.Join(parts, ", ")
}
