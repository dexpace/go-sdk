// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package idempotency

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/dexpace/go-sdk/pipeline"
)

// newUUIDv4 returns a random RFC 4122 version-4 UUID in canonical string form.
// It reads 16 bytes from crypto/rand and returns a wrapped error on failure,
// never a weak or empty key.
func newUUIDv4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("idempotency: read random bytes: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	var buf [36]byte
	hex.Encode(buf[0:8], b[0:4])
	buf[8] = '-'
	hex.Encode(buf[9:13], b[4:6])
	buf[13] = '-'
	hex.Encode(buf[14:18], b[6:8])
	buf[18] = '-'
	hex.Encode(buf[19:23], b[8:10])
	buf[23] = '-'
	hex.Encode(buf[24:36], b[10:16])
	return string(buf[:]), nil
}

const defaultHeader = "Idempotency-Key"

// Options configures the idempotency [Policy]. The zero value is valid and
// yields the documented defaults: POST only, the "Idempotency-Key" header, and
// crypto/rand UUIDv4 keys.
type Options struct {
	// Methods lists the HTTP methods that receive a key. Nil selects ["POST"].
	// Method names are normalised to uppercase before matching, so "post" and
	// http.MethodPost are equivalent.
	Methods []string

	// Header is the header name to set. Empty selects "Idempotency-Key".
	Header string

	// NewKey generates a key. Nil selects a crypto/rand UUIDv4 generator.
	NewKey func() (string, error)
}

// Policy stamps an idempotency-key header on matching requests. It implements
// pipeline.Policy and is safe for concurrent use.
type Policy struct {
	methods map[string]struct{}
	header  string
	newKey  func() (string, error)
}

// NewPolicy returns an idempotency policy configured by opts.
func NewPolicy(opts Options) *Policy {
	methods := opts.Methods
	if methods == nil {
		methods = []string{http.MethodPost}
	}
	set := make(map[string]struct{}, len(methods))
	for _, m := range methods {
		set[strings.ToUpper(m)] = struct{}{}
	}
	h := opts.Header
	if h == "" {
		h = defaultHeader
	}
	newKey := opts.NewKey
	if newKey == nil {
		newKey = newUUIDv4
	}
	return &Policy{methods: set, header: h, newKey: newKey}
}

// Do stamps the configured header with a freshly generated key when the request
// uses a configured method and carries no such header, then marks the request
// idempotent so the retry policy may safely re-send it. Requests using other
// methods pass through untouched. A key-generation failure aborts the request
// without sending it. It implements pipeline.Policy.
func (p *Policy) Do(req *pipeline.Request) (*http.Response, error) {
	raw := req.Raw()
	if _, ok := p.methods[strings.ToUpper(raw.Method)]; !ok {
		return req.Next()
	}
	if raw.Header.Get(p.header) == "" {
		key, err := p.newKey()
		if err != nil {
			return nil, err
		}
		raw.Header.Set(p.header, key)
	}
	pipeline.MarkIdempotent(req)
	return req.Next()
}
