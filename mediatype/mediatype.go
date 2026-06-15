// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package mediatype models HTTP media types (a.k.a. MIME types / content types)
// as immutable values, wrapping the standard library's mime parser and offering
// constants for the types the SDK commonly emits.
package mediatype

import (
	"fmt"
	"mime"
	"strings"
)

// MediaType is an immutable, parsed media type such as "application/json" or
// "text/plain; charset=utf-8". The zero value is the empty media type, whose
// [MediaType.String] is "".
//
// Type and Subtype are exported because their zero values are meaningful and
// direct reads are convenient; parameters are held privately and read through
// [MediaType.Parameter] so the internal map cannot be mutated by callers.
type MediaType struct {
	// Type is the lower-cased top-level type (e.g. "application").
	Type string
	// Subtype is the lower-cased subtype (e.g. "json").
	Subtype string
	// params holds parameters with lower-cased keys; never mutated after build.
	params map[string]string
}

// Parse parses a media type string, optionally with parameters, per RFC 1521.
// The returned type, subtype, and parameter names are lower-cased.
func Parse(s string) (MediaType, error) {
	essence, params, err := mime.ParseMediaType(s)
	if err != nil {
		return MediaType{}, fmt.Errorf("mediatype: parse %q: %w", s, err)
	}
	typ, sub, _ := strings.Cut(essence, "/")
	return MediaType{Type: typ, Subtype: sub, params: params}, nil
}

// WithParameter returns a copy of m with the named parameter set to value. The
// receiver is not modified, preserving immutability.
func (m MediaType) WithParameter(name, value string) MediaType {
	params := make(map[string]string, len(m.params)+1)
	for k, v := range m.params {
		params[k] = v
	}
	params[strings.ToLower(name)] = value
	return MediaType{Type: m.Type, Subtype: m.Subtype, params: params}
}

// Parameter returns the value of the named parameter (case-insensitive) and
// whether it was present.
func (m MediaType) Parameter(name string) (string, bool) {
	v, ok := m.params[strings.ToLower(name)]
	return v, ok
}

// Charset is a convenience accessor for the "charset" parameter; it returns ""
// when absent.
func (m MediaType) Charset() string {
	v, _ := m.Parameter("charset")
	return v
}

// Essence returns the "type/subtype" form without parameters, or "" for the
// zero value.
func (m MediaType) Essence() string {
	if m.Type == "" && m.Subtype == "" {
		return ""
	}
	return m.Type + "/" + m.Subtype
}

// Matches reports whether m and other have the same type and subtype, ignoring
// parameters.
func (m MediaType) Matches(other MediaType) bool {
	return m.Type == other.Type && m.Subtype == other.Subtype
}

// String renders the media type with its parameters, suitable for a
// Content-Type header. It returns "" for the zero value.
func (m MediaType) String() string {
	essence := m.Essence()
	if essence == "" {
		return ""
	}
	if len(m.params) == 0 {
		return essence
	}
	return mime.FormatMediaType(essence, m.params)
}

// Common media types emitted by the SDK. Treat these as constants; do not mutate
// their fields.
var (
	ApplicationJSON           = MediaType{Type: "application", Subtype: "json"}
	ApplicationXML            = MediaType{Type: "application", Subtype: "xml"}
	ApplicationOctetStream    = MediaType{Type: "application", Subtype: "octet-stream"}
	ApplicationFormURLEncoded = MediaType{Type: "application", Subtype: "x-www-form-urlencoded"}
	TextPlain                 = MediaType{Type: "text", Subtype: "plain"}
	TextEventStream           = MediaType{Type: "text", Subtype: "event-stream"}
	MultipartFormData         = MediaType{Type: "multipart", Subtype: "form-data"}
)
