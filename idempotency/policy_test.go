// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package idempotency

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/dexpace/go-sdk/pipeline"
)

var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewUUIDv4Format(t *testing.T) {
	t.Parallel()

	for i := 0; i < 100; i++ {
		got, err := newUUIDv4()
		if err != nil {
			t.Fatalf("newUUIDv4: %v", err)
		}
		if !uuidV4.MatchString(got) {
			t.Fatalf("newUUIDv4 = %q, not a canonical UUIDv4", got)
		}
	}
}

func TestNewUUIDv4Unique(t *testing.T) {
	t.Parallel()

	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		got, err := newUUIDv4()
		if err != nil {
			t.Fatalf("newUUIDv4: %v", err)
		}
		if _, dup := seen[got]; dup {
			t.Fatalf("duplicate UUID %q", got)
		}
		seen[got] = struct{}{}
	}
}

type transporterFunc func(*http.Request) (*http.Response, error)

func (f transporterFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

func okResp(req *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
}

func runPolicy(t *testing.T, p *Policy, req *http.Request) *http.Request {
	t.Helper()
	var captured *http.Request
	tr := transporterFunc(func(r *http.Request) (*http.Response, error) {
		captured = r
		return okResp(r)
	})
	pl := pipeline.New(tr, p)
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return captured
}

func TestPolicyStampsKeyOnPost(t *testing.T) {
	t.Parallel()

	p := NewPolicy(Options{})
	req, _ := http.NewRequest(http.MethodPost, "https://example.test/", strings.NewReader("x"))
	got := runPolicy(t, p, req)

	if key := got.Header.Get("Idempotency-Key"); !uuidV4.MatchString(key) {
		t.Fatalf("Idempotency-Key = %q, want a UUIDv4", key)
	}
}

func TestPolicySkipsGet(t *testing.T) {
	t.Parallel()

	p := NewPolicy(Options{})
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	got := runPolicy(t, p, req)

	if key := got.Header.Get("Idempotency-Key"); key != "" {
		t.Fatalf("Idempotency-Key = %q on GET, want empty", key)
	}
}

func TestPolicyKeepsCallerKey(t *testing.T) {
	t.Parallel()

	p := NewPolicy(Options{})
	var captured *http.Request
	var marked bool
	probe := pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		captured = req.Raw()
		marked = pipeline.IsIdempotent(req)
		return req.Next()
	})
	tr := transporterFunc(okResp)
	pl := pipeline.New(tr, p, probe)
	req, _ := http.NewRequest(http.MethodPost, "https://example.test/", strings.NewReader("x"))
	req.Header.Set("Idempotency-Key", "caller-supplied")
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if key := captured.Header.Get("Idempotency-Key"); key != "caller-supplied" {
		t.Fatalf("Idempotency-Key = %q, want caller-supplied", key)
	}
	if !marked {
		t.Fatal("caller-keyed request not marked idempotent")
	}
}

func TestPolicyMarksRequestIdempotent(t *testing.T) {
	t.Parallel()

	p := NewPolicy(Options{})
	var marked bool
	probe := pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		marked = pipeline.IsIdempotent(req)
		return req.Next()
	})
	tr := transporterFunc(okResp)
	pl := pipeline.New(tr, p, probe)
	req, _ := http.NewRequest(http.MethodPost, "https://example.test/", strings.NewReader("x"))
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if !marked {
		t.Fatal("request not marked idempotent")
	}
}

func TestPolicyKeyGenerationFailure(t *testing.T) {
	t.Parallel()

	p := NewPolicy(Options{NewKey: func() (string, error) {
		return "", errors.New("rng down")
	}})
	tr := transporterFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("transport reached despite key-generation failure")
		return nil, nil
	})
	pl := pipeline.New(tr, p)
	req, _ := http.NewRequest(http.MethodPost, "https://example.test/", strings.NewReader("x"))
	if _, err := pl.Do(req); err == nil {
		t.Fatal("expected error from key-generation failure")
	}
}

func TestPolicyCustomMethods(t *testing.T) {
	t.Parallel()

	p := NewPolicy(Options{Methods: []string{"PATCH"}})

	patch, _ := http.NewRequest(http.MethodPatch, "https://example.test/", strings.NewReader("x"))
	if key := runPolicy(t, p, patch).Header.Get("Idempotency-Key"); !uuidV4.MatchString(key) {
		t.Fatalf("PATCH Idempotency-Key = %q, want a UUIDv4", key)
	}

	// POST is no longer in the configured set, so it must be left untouched.
	post, _ := http.NewRequest(http.MethodPost, "https://example.test/", strings.NewReader("x"))
	if key := runPolicy(t, p, post).Header.Get("Idempotency-Key"); key != "" {
		t.Fatalf("POST Idempotency-Key = %q, want empty when only PATCH is configured", key)
	}
}

func TestPolicyCustomHeader(t *testing.T) {
	t.Parallel()

	p := NewPolicy(Options{Header: "X-Request-ID"})
	req, _ := http.NewRequest(http.MethodPost, "https://example.test/", strings.NewReader("x"))
	got := runPolicy(t, p, req)

	if key := got.Header.Get("X-Request-ID"); !uuidV4.MatchString(key) {
		t.Fatalf("X-Request-ID = %q, want a UUIDv4", key)
	}
	if def := got.Header.Get("Idempotency-Key"); def != "" {
		t.Fatalf("default Idempotency-Key = %q, want empty when custom header configured", def)
	}
}
