// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package auth

import (
	"crypto/md5" //nolint:gosec // G501: MD5 is mandated by RFC 7616 Digest; it is a protocol primitive here, not used for security-sensitive hashing.
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/dexpace/go-sdk/header"
	"github.com/dexpace/go-sdk/pipeline"
)

// DigestAuthPolicy authenticates requests with HTTP Digest Access Authentication
// (RFC 7616). On a 401 response carrying a "WWW-Authenticate: Digest" challenge it
// computes the digest response and retries the request once, then reuses the
// challenge preemptively (with an incrementing nonce count) on later requests
// until the server issues a new nonce. It supports the MD5 and SHA-256 algorithms
// and their "-sess" variants, with qop=auth or no qop.
//
// Like the other credential policies it requires HTTPS and returns
// [ErrInsecureTransport] otherwise; the username and a replayable response hash
// still travel in the header, so the guard is kept for consistency. qop=auth-int,
// SHA-512-256, userhash, and multi-scheme single-header challenges are not
// supported. It implements pipeline.Policy and is safe for concurrent use.
type DigestAuthPolicy struct {
	cred      BasicCredential
	newCnonce func() (string, error)

	mu        sync.Mutex
	challenge *digestChallenge
	nc        uint64
}

// NewDigestAuthPolicy returns a Digest auth policy for the given credentials.
func NewDigestAuthPolicy(cred BasicCredential) *DigestAuthPolicy {
	return &DigestAuthPolicy{cred: cred, newCnonce: randomCnonce}
}

// Do implements pipeline.Policy.
func (p *DigestAuthPolicy) Do(req *pipeline.Request) (*http.Response, error) {
	raw := req.Raw()
	if raw.URL == nil || raw.URL.Scheme != "https" {
		return nil, ErrInsecureTransport
	}

	if ch, nc := p.preempt(); ch != nil && raw.Header.Get(header.Authorization) == "" {
		hdr, err := p.authorization(ch, nc, raw.Method, raw.URL.RequestURI())
		if err != nil {
			return nil, err
		}
		raw.Header.Set(header.Authorization, hdr)
	}

	resp, err := req.Next()
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		return resp, err
	}

	ch := parseChallenge(resp.Header.Values(header.WWWAuthenticate))
	if ch == nil {
		return resp, nil
	}
	if rerr := req.RewindBody(); rerr != nil {
		return resp, nil //nolint:nilerr // intentional: a non-replayable body cannot be retried, so the 401 response is surfaced unchanged.
	}
	nc := p.adopt(ch)
	hdr, herr := p.authorization(ch, nc, raw.Method, raw.URL.RequestURI())
	if herr != nil {
		drainClose(resp)
		return nil, herr
	}
	drainClose(resp)
	raw.Header.Set(header.Authorization, hdr)
	return req.Next()
}

// preempt returns the cached challenge and the next nonce count, or (nil, 0) if no
// challenge has been seen yet.
func (p *DigestAuthPolicy) preempt() (*digestChallenge, uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.challenge == nil {
		return nil, 0
	}
	p.nc++
	return p.challenge, p.nc
}

// adopt records ch as the current challenge (resetting the nonce count when the
// nonce changes) and returns the next nonce count.
func (p *DigestAuthPolicy) adopt(ch *digestChallenge) uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.challenge == nil || p.challenge.nonce != ch.nonce {
		p.challenge = ch
		p.nc = 0
	}
	p.nc++
	return p.nc
}

// digestChallenge is a parsed "WWW-Authenticate: Digest" challenge.
type digestChallenge struct {
	realm       string
	nonce       string
	opaque      string
	algorithm   string // echoed verbatim, e.g. "MD5", "SHA-256", "SHA-256-sess"
	qopAuth     bool
	sess        bool
	hashFactory func() hash.Hash
}

// parseChallenge returns the Digest challenge from the WWW-Authenticate header
// value(s), or nil if none is present or usable. Only one scheme per header value
// is recognised.
func parseChallenge(values []string) *digestChallenge {
	for _, v := range values {
		rest, ok := cutScheme(v)
		if !ok {
			continue
		}
		params := parseAuthParams(rest)
		realm, hasRealm := params["realm"]
		nonce, hasNonce := params["nonce"]
		if !hasRealm || !hasNonce {
			continue
		}
		factory, sess, supported := hashFor(params["algorithm"])
		if !supported {
			continue
		}
		ch := &digestChallenge{
			realm:       realm,
			nonce:       nonce,
			opaque:      params["opaque"],
			algorithm:   params["algorithm"],
			sess:        sess,
			hashFactory: factory,
		}
		for _, opt := range strings.Split(params["qop"], ",") {
			if strings.TrimSpace(opt) == "auth" {
				ch.qopAuth = true
			}
		}
		return ch
	}
	return nil
}

// hashFor selects the hash factory for a Digest algorithm token. An empty token
// means MD5. The bool reports whether the algorithm is supported.
func hashFor(algorithm string) (func() hash.Hash, bool, bool) {
	switch strings.ToUpper(algorithm) {
	case "", "MD5":
		return md5.New, false, true
	case "MD5-SESS":
		return md5.New, true, true
	case "SHA-256":
		return sha256.New, false, true
	case "SHA-256-SESS":
		return sha256.New, true, true
	default:
		return nil, false, false
	}
}

// cutScheme strips a leading "Digest" auth-scheme token (case-insensitive) and
// returns the remaining challenge parameters.
func cutScheme(v string) (string, bool) {
	v = strings.TrimSpace(v)
	const scheme = "digest"
	if len(v) <= len(scheme) || !strings.EqualFold(v[:len(scheme)], scheme) {
		return "", false
	}
	rest := v[len(scheme):]
	if rest[0] != ' ' && rest[0] != '\t' {
		return "", false
	}
	return strings.TrimSpace(rest), true
}

// parseAuthParams scans comma-separated key=value auth parameters, honouring
// double-quoted values (so commas inside a quoted value, as in qop="auth,auth-int"
// or domain="/a,/b", are preserved). Keys are lower-cased.
func parseAuthParams(s string) map[string]string {
	m := make(map[string]string)
	i, n := 0, len(s)
	for i < n {
		for i < n && (s[i] == ' ' || s[i] == '\t' || s[i] == ',') {
			i++
		}
		start := i
		for i < n && s[i] != '=' && s[i] != ',' {
			i++
		}
		key := strings.ToLower(strings.TrimSpace(s[start:i]))
		if i >= n || s[i] == ',' {
			if key != "" {
				m[key] = ""
			}
			continue
		}
		i++ // consume '='
		if i < n && s[i] == '"' {
			i++
			var b strings.Builder
			for i < n && s[i] != '"' {
				if s[i] == '\\' && i+1 < n {
					i++
				}
				b.WriteByte(s[i])
				i++
			}
			if i < n {
				i++ // consume closing quote
			}
			m[key] = b.String()
		} else {
			vstart := i
			for i < n && s[i] != ',' {
				i++
			}
			m[key] = strings.TrimSpace(s[vstart:i])
		}
	}
	return m
}

// authorization builds the Authorization header value for a request.
func (p *DigestAuthPolicy) authorization(ch *digestChallenge, nc uint64, method, uri string) (string, error) {
	cnonce, err := p.newCnonce()
	if err != nil {
		return "", err
	}
	ha1 := hashHex(ch.hashFactory, p.cred.Username+":"+ch.realm+":"+p.cred.Password)
	if ch.sess {
		ha1 = hashHex(ch.hashFactory, ha1+":"+ch.nonce+":"+cnonce)
	}
	ha2 := hashHex(ch.hashFactory, method+":"+uri)

	ncHex := fmt.Sprintf("%08x", nc)
	var response string
	if ch.qopAuth {
		response = hashHex(ch.hashFactory, strings.Join([]string{ha1, ch.nonce, ncHex, cnonce, "auth", ha2}, ":"))
	} else {
		response = hashHex(ch.hashFactory, ha1+":"+ch.nonce+":"+ha2)
	}

	parts := []string{
		quoted("username", p.cred.Username),
		quoted("realm", ch.realm),
		quoted("nonce", ch.nonce),
		quoted("uri", uri),
	}
	if ch.algorithm != "" {
		parts = append(parts, "algorithm="+ch.algorithm)
	}
	if ch.qopAuth {
		parts = append(parts, "qop=auth", "nc="+ncHex, quoted("cnonce", cnonce))
	}
	parts = append(parts, quoted("response", response))
	if ch.opaque != "" {
		parts = append(parts, quoted("opaque", ch.opaque))
	}
	return "Digest " + strings.Join(parts, ", "), nil
}

func quoted(key, value string) string {
	return key + `="` + value + `"`
}

func hashHex(factory func() hash.Hash, s string) string {
	h := factory()
	_, _ = io.WriteString(h, s)
	return hex.EncodeToString(h.Sum(nil))
}

func randomCnonce() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("auth: generate cnonce: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// drainClose discards a bounded amount of the response body and closes it, so the
// underlying keep-alive connection can be reused for the retry.
func drainClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()
}
