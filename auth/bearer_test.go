// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package auth_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dexpace/go-sdk/auth"
	"github.com/dexpace/go-sdk/header"
	"github.com/dexpace/go-sdk/pipeline"
)

type transporterFunc func(*http.Request) (*http.Response, error)

func (f transporterFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

// countingCredential records how many times GetToken is called.
type countingCredential struct {
	token string
	exp   time.Time

	mu    sync.Mutex
	calls int
}

func (c *countingCredential) GetToken(context.Context, auth.TokenRequestOptions) (auth.AccessToken, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return auth.AccessToken{Token: c.token, ExpiresOn: c.exp}, nil
}

func TestBearerAttachesHeaderAndCaches(t *testing.T) {
	t.Parallel()

	var seen string
	transport := transporterFunc(func(req *http.Request) (*http.Response, error) {
		seen = req.Header.Get(header.Authorization)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
	})

	cred := &countingCredential{token: "abc123", exp: time.Now().Add(time.Hour)}
	pl := pipeline.New(transport, auth.NewBearerTokenPolicy(cred, "scope/.default"))

	for range 3 {
		req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
		resp, err := pl.Do(req)
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		_ = resp.Body.Close()
	}

	if seen != "Bearer abc123" {
		t.Fatalf("Authorization = %q, want %q", seen, "Bearer abc123")
	}
	if cred.calls != 1 {
		t.Fatalf("GetToken calls = %d, want 1 (token should be cached)", cred.calls)
	}
}

func TestBearerRefusesInsecureScheme(t *testing.T) {
	t.Parallel()

	transport := transporterFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatal("transport should not be reached for an insecure request")
		return nil, nil
	})
	pl := pipeline.New(transport, auth.NewBearerTokenPolicy(auth.StaticToken("x")))

	req, _ := http.NewRequest(http.MethodGet, "http://api.example.test/", nil)
	_, err := pl.Do(req)
	if !errors.Is(err, auth.ErrInsecureTransport) {
		t.Fatalf("err = %v, want ErrInsecureTransport", err)
	}
}

func TestBearerPropagatesCredentialError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("token endpoint down")
	cred := errCredential{err: wantErr}
	pl := pipeline.New(
		transporterFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: http.NoBody, Request: req}, nil
		}),
		auth.NewBearerTokenPolicy(cred),
	)

	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
	if _, err := pl.Do(req); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

func TestBearerSharedCacheReusesToken(t *testing.T) {
	t.Parallel()

	cred := &countingCredential{token: "tok", exp: time.Now().Add(time.Hour)}
	cache := auth.NewInMemoryTokenCache()

	run := func(p *auth.BearerTokenPolicy) {
		transport := transporterFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
		})
		pl := pipeline.New(transport, p)
		req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
		resp, err := pl.Do(req)
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		_ = resp.Body.Close()
	}

	run(auth.NewBearerTokenPolicyWithCache(cred, cache, "scope/.default"))
	run(auth.NewBearerTokenPolicyWithCache(cred, cache, "scope/.default"))

	if cred.calls != 1 {
		t.Fatalf("GetToken calls = %d, want 1 (shared cache reuses the token)", cred.calls)
	}
}

type errCredential struct{ err error }

func (e errCredential) GetToken(context.Context, auth.TokenRequestOptions) (auth.AccessToken, error) {
	return auth.AccessToken{}, e.err
}

func TestBearerRefetchesNearExpiryToken(t *testing.T) {
	t.Parallel()

	// Token expires in one minute — inside the five-minute freshness window, so it
	// is never considered fresh and must be re-fetched on every request.
	cred := &countingCredential{token: "tok", exp: time.Now().Add(time.Minute)}
	pl := pipeline.New(
		transporterFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
		}),
		auth.NewBearerTokenPolicy(cred, "scope"),
	)
	for range 2 {
		req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
		resp, err := pl.Do(req)
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		_ = resp.Body.Close()
	}
	if cred.calls != 2 {
		t.Fatalf("GetToken calls = %d, want 2 (near-expiry token re-fetched each request)", cred.calls)
	}
}
