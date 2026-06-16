// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package instrumentation_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/dexpace/go-sdk/instrumentation"
	"github.com/dexpace/go-sdk/pipeline"
)

type transporterFunc func(*http.Request) (*http.Response, error)

func (f transporterFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

func okResp(req *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
}

type fakeSpan struct {
	attrs []instrumentation.Attr
	errs  []error
	ended bool
	sc    instrumentation.SpanContext
}

func (s *fakeSpan) SetAttributes(a ...instrumentation.Attr) { s.attrs = append(s.attrs, a...) }
func (s *fakeSpan) RecordError(e error)                     { s.errs = append(s.errs, e) }
func (s *fakeSpan) End()                                    { s.ended = true }
func (s *fakeSpan) Context() instrumentation.SpanContext    { return s.sc }

type fakeTracer struct {
	span    *fakeSpan
	started string
}

func (f *fakeTracer) StartSpan(ctx context.Context, name string) (context.Context, instrumentation.Span) {
	f.started = name
	return ctx, f.span
}

func TestTracingPolicyRecordsSuccess(t *testing.T) {
	t.Parallel()

	span := &fakeSpan{}
	tr := &fakeTracer{span: span}
	pl := pipeline.New(transporterFunc(okResp), instrumentation.NewTracingPolicy(tr, nil))

	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/x?token=secret", nil)
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if tr.started != http.MethodGet {
		t.Fatalf("span name = %q, want GET", tr.started)
	}
	if !span.ended {
		t.Fatal("span not ended")
	}
	if len(span.errs) != 0 {
		t.Fatalf("unexpected RecordError calls: %v", span.errs)
	}
	if !hasAttr(span.attrs, "url.full", "https://api.example.test/x?token=REDACTED") {
		t.Fatalf("url.full not redacted: %v", span.attrs)
	}
	if !hasAttrKey(span.attrs, "http.response.status_code") {
		t.Fatalf("status_code attribute missing: %v", span.attrs)
	}
}

func TestTracingPolicyRecordsError(t *testing.T) {
	t.Parallel()

	span := &fakeSpan{}
	tr := &fakeTracer{span: span}
	boom := errors.New("boom")
	pl := pipeline.New(transporterFunc(func(*http.Request) (*http.Response, error) {
		return nil, boom
	}), instrumentation.NewTracingPolicy(tr, nil))

	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/x", nil)
	_, _ = pl.Do(req)

	if !span.ended {
		t.Fatal("span not ended on error")
	}
	if len(span.errs) != 1 || !errors.Is(span.errs[0], boom) {
		t.Fatalf("RecordError = %v, want boom", span.errs)
	}
}

func TestTracingPolicyInjectsTraceparent(t *testing.T) {
	t.Parallel()

	span := &fakeSpan{sc: instrumentation.SpanContext{
		TraceID: [16]byte{0: 0x0a, 15: 0x0b},
		SpanID:  [8]byte{0: 0x01, 7: 0x02},
		Sampled: true,
	}}
	tr := &fakeTracer{span: span}

	var seen string
	pl := pipeline.New(transporterFunc(func(req *http.Request) (*http.Response, error) {
		seen = req.Header.Get("Traceparent")
		return okResp(req)
	}), instrumentation.NewTracingPolicy(tr, nil))

	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/x", nil)
	resp, _ := pl.Do(req)
	t.Cleanup(func() { _ = resp.Body.Close() })

	want := "00-0a00000000000000000000000000000b-0100000000000002-01"
	if seen != want {
		t.Fatalf("traceparent = %q, want %q", seen, want)
	}
}

func TestTracingPolicyDoesNotOverrideTraceparent(t *testing.T) {
	t.Parallel()

	span := &fakeSpan{sc: instrumentation.SpanContext{TraceID: [16]byte{0: 1}, SpanID: [8]byte{0: 1}}}
	tr := &fakeTracer{span: span}

	var seen string
	pl := pipeline.New(transporterFunc(func(req *http.Request) (*http.Response, error) {
		seen = req.Header.Get("Traceparent")
		return okResp(req)
	}), instrumentation.NewTracingPolicy(tr, nil))

	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/x", nil)
	req.Header.Set("Traceparent", "caller-value")
	resp, _ := pl.Do(req)
	t.Cleanup(func() { _ = resp.Body.Close() })

	if seen != "caller-value" {
		t.Fatalf("traceparent = %q, want caller-value (not overridden)", seen)
	}
}

func TestTracingPolicyNoTraceparentWhenZero(t *testing.T) {
	t.Parallel()

	span := &fakeSpan{} // zero SpanContext
	tr := &fakeTracer{span: span}

	var seen string
	pl := pipeline.New(transporterFunc(func(req *http.Request) (*http.Response, error) {
		seen = req.Header.Get("Traceparent")
		return okResp(req)
	}), instrumentation.NewTracingPolicy(tr, nil))

	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/x", nil)
	resp, _ := pl.Do(req)
	t.Cleanup(func() { _ = resp.Body.Close() })

	if seen != "" {
		t.Fatalf("traceparent = %q, want empty for zero span context", seen)
	}
}

func TestTracingPolicySetsMethodAndHostWithoutPort(t *testing.T) {
	t.Parallel()

	span := &fakeSpan{}
	tr := &fakeTracer{span: span}
	pl := pipeline.New(transporterFunc(okResp), instrumentation.NewTracingPolicy(tr, nil))

	req, _ := http.NewRequest(http.MethodPost, "https://api.example.test:8443/x", nil)
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if !hasAttr(span.attrs, "http.request.method", http.MethodPost) {
		t.Fatalf("http.request.method attribute missing or wrong: %v", span.attrs)
	}
	if !hasAttr(span.attrs, "server.address", "api.example.test") {
		t.Fatalf("server.address should be host without port: %v", span.attrs)
	}
}

func hasAttr(attrs []instrumentation.Attr, key string, value any) bool {
	for _, a := range attrs {
		if a.Key == key && a.Value == value {
			return true
		}
	}
	return false
}

func hasAttrKey(attrs []instrumentation.Attr, key string) bool {
	for _, a := range attrs {
		if a.Key == key {
			return true
		}
	}
	return false
}
