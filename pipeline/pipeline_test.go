// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package pipeline_test

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/dexpace/go-sdk/pipeline"
)

// transporterFunc adapts a function to pipeline.Transporter.
type transporterFunc func(*http.Request) (*http.Response, error)

func (f transporterFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

func okResponse(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("ok")),
		Request:    req,
	}, nil
}

func TestPipelineRunsPoliciesInOrder(t *testing.T) {
	t.Parallel()

	var order []string
	mark := func(name string) pipeline.Policy {
		return pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
			order = append(order, name)
			return req.Next()
		})
	}

	transport := transporterFunc(func(req *http.Request) (*http.Response, error) {
		order = append(order, "transport")
		return okResponse(req)
	})

	pl := pipeline.New(transport, mark("a"), mark("b"), mark("c"))

	req, err := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	want := []string{"a", "b", "c", "transport"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("policy order = %v, want %v", order, want)
	}
}

func TestPolicyCanShortCircuit(t *testing.T) {
	t.Parallel()

	reached := false
	gate := pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		// Do not call Next: short-circuit the chain.
		return okResponse(req.Raw())
	})
	sentinel := pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		reached = true
		return req.Next()
	})
	transport := transporterFunc(func(req *http.Request) (*http.Response, error) {
		reached = true
		return okResponse(req)
	})

	pl := pipeline.New(transport, gate, sentinel)
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if reached {
		t.Fatal("downstream policy/transport ran despite short-circuit")
	}
}

func TestNextCanBeCalledRepeatedly(t *testing.T) {
	t.Parallel()

	const attempts = 3
	calls := 0
	repeat := pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		var resp *http.Response
		var err error
		for i := 0; i < attempts; i++ {
			resp, err = req.Next()
		}
		return resp, err
	})
	transport := transporterFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return okResponse(req)
	})

	pl := pipeline.New(transport, repeat)
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if calls != attempts {
		t.Fatalf("transport calls = %d, want %d", calls, attempts)
	}
}

func TestDoRejectsNilRequest(t *testing.T) {
	t.Parallel()

	pl := pipeline.New(transporterFunc(okResponse))
	if _, err := pl.Do(nil); err == nil {
		t.Fatal("expected error for nil request")
	}
}

func TestZeroPipelineDoFails(t *testing.T) {
	t.Parallel()

	var pl pipeline.Pipeline
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	if _, err := pl.Do(req); err == nil {
		t.Fatal("expected error from zero-value pipeline")
	}
}

func TestNewPanicsOnNilTransport(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for nil transport")
		}
	}()
	_ = pipeline.New(nil)
}

func TestRewindBodyReplaysPayload(t *testing.T) {
	t.Parallel()

	var bodies []string
	transport := transporterFunc(func(req *http.Request) (*http.Response, error) {
		b, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		bodies = append(bodies, string(b))
		return okResponse(req)
	})

	replay := pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		resp, err := req.Next()
		if err != nil {
			return nil, err
		}
		_ = resp.Body.Close()
		if err := req.RewindBody(); err != nil {
			return nil, err
		}
		return req.Next()
	})

	pl := pipeline.New(transport, replay)
	req, err := http.NewRequest(http.MethodPost, "https://example.test/", strings.NewReader("payload"))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if len(bodies) != 2 || bodies[0] != "payload" || bodies[1] != "payload" {
		t.Fatalf("bodies = %v, want both \"payload\"", bodies)
	}
}

func TestValuesPropagateDownstream(t *testing.T) {
	t.Parallel()

	type ctxKey struct{}
	producer := pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		req.SetValue(ctxKey{}, "hello")
		return req.Next()
	})
	var got any
	consumer := pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		got = req.Value(ctxKey{})
		return req.Next()
	})

	pl := pipeline.New(transporterFunc(okResponse), producer, consumer)
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if got != "hello" {
		t.Fatalf("downstream value = %v, want \"hello\"", got)
	}
}

func TestErrorsPropagate(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("boom")
	transport := transporterFunc(func(*http.Request) (*http.Response, error) {
		return nil, wantErr
	})
	pl := pipeline.New(transport)
	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	if _, err := pl.Do(req); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}
