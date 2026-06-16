// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package auth_test

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/dexpace/go-sdk/auth"
	"github.com/dexpace/go-sdk/pipeline"
)

func okTransport(seen *http.Request) transporterFunc {
	return func(req *http.Request) (*http.Response, error) {
		*seen = *req
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
	}
}

func TestBasicAuthAttachesHeader(t *testing.T) {
	t.Parallel()

	var seen http.Request
	pl := pipeline.New(okTransport(&seen),
		auth.NewBasicAuthPolicy(auth.BasicCredential{Username: "alice", Password: "s3cr3t"}))
	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	u, p, ok := seen.BasicAuth()
	if !ok || u != "alice" || p != "s3cr3t" {
		t.Fatalf("BasicAuth = (%q,%q,%v), want alice/s3cr3t/true", u, p, ok)
	}
}

func TestBasicAuthRefusesInsecure(t *testing.T) {
	t.Parallel()

	pl := pipeline.New(
		transporterFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("transport reached for insecure request")
			return nil, nil
		}),
		auth.NewBasicAuthPolicy(auth.BasicCredential{Username: "a", Password: "b"}),
	)
	req, _ := http.NewRequest(http.MethodGet, "http://api.example.test/", nil)
	if _, err := pl.Do(req); !errors.Is(err, auth.ErrInsecureTransport) {
		t.Fatalf("err = %v, want ErrInsecureTransport", err)
	}
}

func TestBasicAuthPreservesCallerHeader(t *testing.T) {
	t.Parallel()

	var seen http.Request
	pl := pipeline.New(okTransport(&seen),
		auth.NewBasicAuthPolicy(auth.BasicCredential{Username: "alice", Password: "s3cr3t"}))
	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
	req.Header.Set("Authorization", "Bearer caller-token")
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if got := seen.Header.Get("Authorization"); got != "Bearer caller-token" {
		t.Fatalf("Authorization = %q, want the caller value preserved", got)
	}
}
