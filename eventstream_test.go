// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package dexpace_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	dexpace "github.com/dexpace/go-sdk"
	"github.com/dexpace/go-sdk/retry"
	"github.com/dexpace/go-sdk/sse"
)

// sseStub is a stub Transporter that returns scripted responses and records the
// Last-Event-Id and Accept headers seen on each connect.
type sseStub struct {
	mu           sync.Mutex
	calls        int
	lastEventIDs []string
	accepts      []string
	responses    []func() (*http.Response, error)
}

func (s *sseStub) Do(req *http.Request) (*http.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastEventIDs = append(s.lastEventIDs, req.Header.Get("Last-Event-Id"))
	s.accepts = append(s.accepts, req.Header.Get("Accept"))
	i := s.calls
	s.calls++
	if i < len(s.responses) {
		return s.responses[i]()
	}
	return nil, errors.New("sseStub: no more scripted responses")
}

func sseBody(status int, body string) func() (*http.Response, error) {
	return func() (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Status:     http.StatusText(status),
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	}
}

func newEventStreamClient(stub *sseStub) *dexpace.Client {
	return dexpace.New(
		dexpace.WithTransport(stub),
		dexpace.WithRetry(retry.Options{MaxRetries: -1}), // deterministic connect counts
	)
}

func TestEventStreamReconnectAndFlatten(t *testing.T) {
	t.Parallel()
	stub := &sseStub{responses: []func() (*http.Response, error){
		sseBody(200, "id: 1\ndata: a\n\nid: 2\ndata: b\n\n"),
		sseBody(200, "id: 3\ndata: c\n\n"),
		func() (*http.Response, error) { return nil, errors.New("stop") },
	}}
	client := newEventStreamClient(stub)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.test/stream", nil)
	if err != nil {
		t.Fatal(err)
	}

	var data []string
	var gotErr error
	for ev, err := range client.EventStream(context.Background(), req, sse.WithReconnectDelay(0)) {
		if err != nil {
			gotErr = err
			break
		}
		data = append(data, ev.Data)
	}
	if strings.Join(data, ",") != "a,b,c" {
		t.Fatalf("events = %v, want [a b c]", data)
	}
	if gotErr == nil || !strings.Contains(gotErr.Error(), "stop") {
		t.Fatalf("final error = %v, want the terminal connect error", gotErr)
	}
}

func TestEventStreamLastEventIDAndAccept(t *testing.T) {
	t.Parallel()
	stub := &sseStub{responses: []func() (*http.Response, error){
		sseBody(200, "id: 1\ndata: a\n\nid: 2\ndata: b\n\n"),
		sseBody(200, "id: 3\ndata: c\n\n"),
		func() (*http.Response, error) { return nil, errors.New("stop") },
	}}
	client := newEventStreamClient(stub)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.test/stream", nil)

	for _, err := range client.EventStream(context.Background(), req, sse.WithReconnectDelay(0)) {
		if err != nil {
			break
		}
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.lastEventIDs) < 3 {
		t.Fatalf("got %d connects, want >= 3", len(stub.lastEventIDs))
	}
	if stub.lastEventIDs[0] != "" {
		t.Fatalf("first connect Last-Event-Id = %q, want empty", stub.lastEventIDs[0])
	}
	if stub.lastEventIDs[1] != "2" {
		t.Fatalf("second connect Last-Event-Id = %q, want 2", stub.lastEventIDs[1])
	}
	if stub.lastEventIDs[2] != "3" {
		t.Fatalf("third connect Last-Event-Id = %q, want 3", stub.lastEventIDs[2])
	}
	for i, a := range stub.accepts {
		if a != "text/event-stream" {
			t.Fatalf("connect %d Accept = %q, want text/event-stream", i, a)
		}
	}
}

func TestEventStreamNon2xxIsTerminal(t *testing.T) {
	t.Parallel()
	stub := &sseStub{responses: []func() (*http.Response, error){
		sseBody(503, "unavailable"),
	}}
	client := newEventStreamClient(stub)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.test/stream", nil)

	var events int
	var gotErr error
	for ev, err := range client.EventStream(context.Background(), req, sse.WithReconnectDelay(0)) {
		if err != nil {
			gotErr = err
			break
		}
		_ = ev
		events++
	}
	if events != 0 {
		t.Fatalf("got %d events, want 0 on a non-2xx connect", events)
	}
	if gotErr == nil {
		t.Fatal("want a terminal error on a non-2xx connect")
	}
}
