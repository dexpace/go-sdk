// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package auth

import "testing"

// RFC 7616 §3.9.1 published vectors.
const (
	rfcUser   = "Mufasa"
	rfcPass   = "Circle of Life"
	rfcRealm  = "http-auth@example.org"
	rfcNonce  = "7ypf/xlj9XXwfDPEoM4URrv/xwf94BcCAzFZH4GiTo0v"
	rfcCnonce = "f2/wE4q74E6zIJEtWaHKaf5wv/H5QzzpXusqGemxURZJ"
	rfcURI    = "/dir/index.html"
)

func fixedDigest(t *testing.T) *DigestAuthPolicy {
	t.Helper()
	p := NewDigestAuthPolicy(BasicCredential{Username: rfcUser, Password: rfcPass})
	p.newCnonce = func() (string, error) { return rfcCnonce, nil }
	return p
}

func TestAuthorizationRFC7616SHA256(t *testing.T) {
	t.Parallel()
	p := fixedDigest(t)
	ch := parseChallenge([]string{`Digest realm="` + rfcRealm + `", qop="auth", algorithm=SHA-256, nonce="` + rfcNonce + `", opaque="FQhe/qaU925kfnzjCev0ciny7QMkPqMAFRtzCUYo5tdS"`})
	if ch == nil {
		t.Fatal("parseChallenge returned nil for a SHA-256 challenge")
	}
	hdr, err := p.authorization(ch, 1, "GET", rfcURI)
	if err != nil {
		t.Fatalf("authorization: %v", err)
	}
	const want = "753927fa0e85d155564e2e272a28d1802ca10daf4496794697cf8db5856cb6c1"
	if !containsParam(hdr, "response", want) {
		t.Fatalf("SHA-256 response mismatch.\nheader: %s\nwant response=%q", hdr, want)
	}
}

func TestAuthorizationRFC7616MD5(t *testing.T) {
	t.Parallel()
	p := fixedDigest(t)
	ch := parseChallenge([]string{`Digest realm="` + rfcRealm + `", qop="auth", algorithm=MD5, nonce="` + rfcNonce + `", opaque="FQhe/qaU925kfnzjCev0ciny7QMkPqMAFRtzCUYo5tdS"`})
	if ch == nil {
		t.Fatal("parseChallenge returned nil for an MD5 challenge")
	}
	hdr, err := p.authorization(ch, 1, "GET", rfcURI)
	if err != nil {
		t.Fatalf("authorization: %v", err)
	}
	const want = "8ca523f5e9506fed4657c9700eebdbec"
	if !containsParam(hdr, "response", want) {
		t.Fatalf("MD5 response mismatch.\nheader: %s\nwant response=%q", hdr, want)
	}
}

func TestParseChallengeFields(t *testing.T) {
	t.Parallel()
	ch := parseChallenge([]string{`Digest realm="r", domain="/a,/b", qop="auth,auth-int", nonce="n", opaque="o", algorithm=SHA-256`})
	if ch == nil {
		t.Fatal("nil challenge")
	}
	if ch.realm != "r" || ch.nonce != "n" || ch.opaque != "o" || !ch.qopAuth || ch.sess {
		t.Fatalf("bad parse: %+v", ch)
	}
}

func TestParseChallengeIgnoresNonDigest(t *testing.T) {
	t.Parallel()
	if ch := parseChallenge([]string{`Basic realm="r"`}); ch != nil {
		t.Fatalf("expected nil for a non-Digest scheme, got %+v", ch)
	}
	if ch := parseChallenge([]string{`Digest realm="r"`}); ch != nil {
		t.Fatal("expected nil when nonce is missing")
	}
}

// containsParam reports whether the Digest header contains key="value" (quoted) or
// key=value (token).
func containsParam(hdr, key, value string) bool {
	params := parseAuthParams(hdr[len("Digest "):])
	return params[key] == value
}
