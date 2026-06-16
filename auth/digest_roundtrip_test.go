// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package auth_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dexpace/go-sdk/auth"
	"github.com/dexpace/go-sdk/pipeline"
)

// sha256Hex hashes s and returns the lowercase hex digest, matching the server
// side of an RFC 7616 SHA-256 exchange.
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// digestParam extracts a single auth-param from a "Digest ..." header value.
// Quoted and token forms are both handled.
func digestParam(hdr, key string) string {
	rest := strings.TrimPrefix(hdr, "Digest ")
	for _, part := range splitAuthParams(rest) {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(k), key) {
			return strings.Trim(strings.TrimSpace(v), `"`)
		}
	}
	return ""
}

// splitAuthParams splits a comma-separated parameter list, keeping commas inside
// double-quoted values intact.
func splitAuthParams(s string) []string {
	var parts []string
	var b strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			inQuote = !inQuote
			b.WriteByte(c)
		case c == ',' && !inQuote:
			parts = append(parts, b.String())
			b.Reset()
		default:
			b.WriteByte(c)
		}
	}
	if b.Len() > 0 {
		parts = append(parts, b.String())
	}
	return parts
}

// expectedResponse recomputes the RFC 7616 SHA-256 qop=auth response for the
// server-known credentials.
func expectedResponse(username, password, realm, nonce, uri, nc, cnonce string) string {
	ha1 := sha256Hex(username + ":" + realm + ":" + password)
	ha2 := sha256Hex("GET:" + uri)
	return sha256Hex(strings.Join([]string{ha1, nonce, nc, cnonce, "auth", ha2}, ":"))
}

func TestDigestRoundTrip(t *testing.T) {
	t.Parallel()

	const (
		user   = "u"
		pass   = "pw"
		realm  = "test"
		nonce  = "abc123"
		opaque = "xyz"
	)

	var (
		challenges  atomic.Int64 // count of 401 challenges issued
		successes   atomic.Int64 // count of 200 responses
		firstHadCtl atomic.Bool  // whether the very first request carried Authorization
		seenRequest atomic.Int64 // total requests seen
	)

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := seenRequest.Add(1)
		authz := r.Header.Get("Authorization")
		if n == 1 {
			firstHadCtl.Store(authz != "")
		}
		if !strings.HasPrefix(authz, "Digest ") {
			challenges.Add(1)
			w.Header().Set("WWW-Authenticate",
				fmt.Sprintf(`Digest realm=%q, qop="auth", nonce=%q, algorithm=SHA-256, opaque=%q`, realm, nonce, opaque))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		uri := digestParam(authz, "uri")
		nc := digestParam(authz, "nc")
		cnonce := digestParam(authz, "cnonce")
		got := digestParam(authz, "response")
		want := expectedResponse(user, pass, realm, nonce, uri, nc, cnonce)
		if got != want {
			challenges.Add(1)
			w.Header().Set("WWW-Authenticate",
				fmt.Sprintf(`Digest realm=%q, qop="auth", nonce=%q, algorithm=SHA-256, opaque=%q`, realm, nonce, opaque))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		successes.Add(1)
		w.Header().Set("X-Echo-NC", nc)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(srv.Close)

	// *http.Client satisfies pipeline.Transporter (Do(*http.Request) (*http.Response, error)).
	policy := auth.NewDigestAuthPolicy(auth.BasicCredential{Username: user, Password: pass})
	pl := pipeline.New(srv.Client(), policy)

	// First request: server challenges once, policy retries with the digest, 200.
	req1, _ := http.NewRequest(http.MethodGet, srv.URL+"/dir/index.html", nil)
	resp1, err := pl.Do(req1)
	if err != nil {
		t.Fatalf("first Do: %v", err)
	}
	t.Cleanup(func() { _ = resp1.Body.Close() })
	body1, _ := io.ReadAll(resp1.Body)
	if resp1.StatusCode != http.StatusOK || string(body1) != "ok" {
		t.Fatalf("first response = %d %q, want 200 \"ok\"", resp1.StatusCode, body1)
	}
	if got := challenges.Load(); got != 1 {
		t.Fatalf("challenges after first request = %d, want exactly 1", got)
	}
	if firstHadCtl.Load() {
		t.Fatal("first request unexpectedly carried Authorization (no challenge cached yet)")
	}
	if got := resp1.Header.Get("X-Echo-NC"); got != "00000001" {
		t.Fatalf("first success nc = %q, want 00000001", got)
	}

	// Second request through the SAME policy: the challenge is reused preemptively,
	// so the very first hit carries Authorization and increments nc to 00000002.
	beforeChallenges := challenges.Load()
	req2, _ := http.NewRequest(http.MethodGet, srv.URL+"/dir/index.html", nil)
	resp2, err := pl.Do(req2)
	if err != nil {
		t.Fatalf("second Do: %v", err)
	}
	t.Cleanup(func() { _ = resp2.Body.Close() })
	body2, _ := io.ReadAll(resp2.Body)
	if resp2.StatusCode != http.StatusOK || string(body2) != "ok" {
		t.Fatalf("second response = %d %q, want 200 \"ok\"", resp2.StatusCode, body2)
	}
	if got := challenges.Load(); got != beforeChallenges {
		t.Fatalf("second request triggered %d new challenge(s), want 0 (preemptive auth)", got-beforeChallenges)
	}
	if got := resp2.Header.Get("X-Echo-NC"); got != "00000002" {
		t.Fatalf("second success nc = %q, want 00000002 (incrementing nonce count)", got)
	}
}
