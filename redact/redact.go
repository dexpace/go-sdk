// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package redact

import (
	"net/url"
	"sort"
	"strings"
)

// redactedValue replaces a non-allowlisted query-param value.
const redactedValue = "REDACTED"

// Redactor renders URLs with userinfo stripped and non-allowlisted query-param
// values replaced by "REDACTED". A Redactor is safe for concurrent use.
type Redactor struct {
	allowed map[string]struct{}
}

// New returns a Redactor that preserves the values of the named query parameters
// and redacts all others. With no names, every query value is redacted
// (default-deny).
func New(allowedQueryParams ...string) *Redactor {
	allowed := make(map[string]struct{}, len(allowedQueryParams))
	for _, p := range allowedQueryParams {
		allowed[p] = struct{}{}
	}
	return &Redactor{allowed: allowed}
}

// Default is the default-deny redactor: it redacts every query-param value.
var Default = New()

// URL returns a redacted string form of u: userinfo removed and every
// non-allowlisted query-param value replaced with "REDACTED". Keys, path, and
// fragment are preserved. A nil URL yields "".
func (r *Redactor) URL(u *url.URL) string {
	if u == nil {
		return ""
	}
	c := *u
	c.User = nil
	if c.RawQuery != "" {
		c.RawQuery = r.redactQuery(c.Query())
	}
	return c.String()
}

// redactQuery rebuilds a query string with non-allowlisted values redacted,
// ordered by key for determinism.
func (r *Redactor) redactQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		_, allow := r.allowed[k]
		for _, v := range q[k] {
			if b.Len() > 0 {
				b.WriteByte('&')
			}
			b.WriteString(url.QueryEscape(k))
			b.WriteByte('=')
			if allow {
				b.WriteString(url.QueryEscape(v))
			} else {
				b.WriteString(redactedValue)
			}
		}
	}
	return b.String()
}
