// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package logging_test

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/dexpace/go-sdk/logging"
	"github.com/dexpace/go-sdk/pipeline"
)

type transporterFunc func(*http.Request) (*http.Response, error)

func (f transporterFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

func TestLoggingRedactsQuerySecret(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	transport := transporterFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
	})
	pl := pipeline.New(transport, logging.NewPolicy(logging.Options{Logger: logger}))

	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/x?token=secret", nil)
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	out := buf.String()
	if strings.Contains(out, "secret") {
		t.Fatalf("log leaked the query secret: %s", out)
	}
	if !strings.Contains(out, "token=REDACTED") {
		t.Fatalf("log should show token=REDACTED: %s", out)
	}
}
