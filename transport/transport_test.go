// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package transport_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dexpace/go-sdk/transport"
)

func TestWithMaxRedirectsZeroDoesNotFollow(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/dest", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	tr := transport.New(transport.WithMaxRedirects(0))
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/start", nil)
	resp, err := tr.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302 (redirect not followed)", resp.StatusCode)
	}
}

func TestWithMaxRedirectsCaps(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/next", http.StatusFound)
	}))
	t.Cleanup(srv.Close)

	tr := transport.New(transport.WithMaxRedirects(2))
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/start", nil)
	resp, err := tr.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("expected error after exceeding redirect cap")
	}
}

func TestWithRedirectPolicyCustom(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/dest", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusTeapot)
	}))
	t.Cleanup(srv.Close)

	var hops int
	tr := transport.New(transport.WithRedirectPolicy(func(req *http.Request, via []*http.Request) error {
		hops = len(via)
		return nil // follow
	}))
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/start", nil)
	resp, err := tr.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusTeapot {
		t.Fatalf("status = %d, want 418 (redirect followed)", resp.StatusCode)
	}
	if hops != 1 {
		t.Fatalf("custom policy saw %d prior requests, want 1", hops)
	}
}

func TestWithClientBypassesRedirectOptions(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/dest", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusTeapot)
	}))
	t.Cleanup(srv.Close)

	// A supplied client follows redirects by default. WithMaxRedirects(0) would
	// otherwise stop following, but WithClient takes precedence and the option
	// is ignored.
	tr := transport.New(
		transport.WithClient(&http.Client{}),
		transport.WithMaxRedirects(0),
	)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/start", nil)
	resp, err := tr.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusTeapot {
		t.Fatalf("status = %d, want 418 (supplied client followed the redirect despite WithMaxRedirects(0))", resp.StatusCode)
	}
}
