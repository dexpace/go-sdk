// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package auth_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/dexpace/go-sdk/auth"
	"github.com/dexpace/go-sdk/pipeline"
)

func TestAPIKeyAttachesHeader(t *testing.T) {
	t.Parallel()

	var seen http.Request
	pl := pipeline.New(okTransport(&seen), auth.NewAPIKeyPolicy("X-API-Key", "secret-key"))
	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if got := seen.Header.Get("X-API-Key"); got != "secret-key" {
		t.Fatalf("X-API-Key = %q, want secret-key", got)
	}
}

func TestAPIKeyRefusesInsecure(t *testing.T) {
	t.Parallel()

	pl := pipeline.New(
		transporterFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("transport reached for insecure request")
			return nil, nil
		}),
		auth.NewAPIKeyPolicy("X-API-Key", "secret-key"),
	)
	req, _ := http.NewRequest(http.MethodGet, "http://api.example.test/", nil)
	if _, err := pl.Do(req); !errors.Is(err, auth.ErrInsecureTransport) {
		t.Fatalf("err = %v, want ErrInsecureTransport", err)
	}
}

func TestAPIKeyPreservesCallerHeader(t *testing.T) {
	t.Parallel()

	var seen http.Request
	pl := pipeline.New(okTransport(&seen), auth.NewAPIKeyPolicy("X-API-Key", "secret-key"))
	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/", nil)
	req.Header.Set("X-API-Key", "caller-key")
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if got := seen.Header.Get("X-API-Key"); got != "caller-key" {
		t.Fatalf("X-API-Key = %q, want the caller value preserved", got)
	}
}

func TestNewAPIKeyPolicyPanicsOnEmptyHeader(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for empty header name")
		}
	}()
	_ = auth.NewAPIKeyPolicy("", "key")
}
