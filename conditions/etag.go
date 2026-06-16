// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package conditions

import (
	"fmt"
	"strings"
)

// ETag is an HTTP entity-tag validator (RFC 9110). The tag is the opaque value
// without surrounding quotes; a weak tag is rendered with a leading "W/".
type ETag struct {
	tag  string
	weak bool
}

// NewETag returns a strong entity tag.
func NewETag(tag string) ETag { return ETag{tag: tag} }

// NewWeakETag returns a weak entity tag.
func NewWeakETag(tag string) ETag { return ETag{tag: tag, weak: true} }

// Parse parses an entity tag in wire form, "abc" or W/"abc", returning an error
// for input that is not a (optionally W/-prefixed) quoted string.
func Parse(s string) (ETag, error) {
	weak := false
	if rest, ok := strings.CutPrefix(s, "W/"); ok {
		weak = true
		s = rest
	}
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return ETag{}, fmt.Errorf("conditions: invalid ETag %q", s)
	}
	return ETag{tag: s[1 : len(s)-1], weak: weak}, nil
}

// Tag returns the opaque tag value without quotes.
func (e ETag) Tag() string { return e.tag }

// Weak reports whether the tag is a weak validator.
func (e ETag) Weak() bool { return e.weak }

// String returns the wire form: "abc" for a strong tag, W/"abc" for a weak one.
func (e ETag) String() string {
	quoted := `"` + e.tag + `"`
	if e.weak {
		return "W/" + quoted
	}
	return quoted
}
