// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package sse_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/dexpace/go-sdk/sse"
)

func nopBody(s string) io.ReadCloser { return io.NopCloser(strings.NewReader(s)) }

func TestStreamReconnectsAndFlattens(t *testing.T) {
	t.Parallel()

	calls := 0
	connect := func(_ context.Context, _ string) (io.ReadCloser, error) {
		calls++
		switch calls {
		case 1:
			return nopBody("data: a\n\ndata: b\n\n"), nil
		case 2:
			return nopBody("data: c\n\n"), nil
		default:
			return nil, errors.New("stop")
		}
	}

	var data []string
	var gotErr error
	for ev, err := range sse.Stream(context.Background(), connect, sse.WithReconnectDelay(0)) {
		if err != nil {
			gotErr = err
			break
		}
		data = append(data, ev.Data)
	}

	if strings.Join(data, ",") != "a,b,c" {
		t.Fatalf("data = %v, want [a b c]", data)
	}
	if gotErr == nil || gotErr.Error() != "stop" {
		t.Fatalf("err = %v, want stop", gotErr)
	}
}

func TestStreamReplaysLastEventID(t *testing.T) {
	t.Parallel()

	var seenIDs []string
	calls := 0
	connect := func(_ context.Context, lastID string) (io.ReadCloser, error) {
		seenIDs = append(seenIDs, lastID)
		calls++
		if calls == 1 {
			return nopBody("id: 42\ndata: a\n\n"), nil
		}
		return nil, errors.New("stop")
	}

	for _, err := range sse.Stream(context.Background(), connect, sse.WithReconnectDelay(0)) {
		if err != nil {
			break
		}
	}

	if len(seenIDs) != 2 || seenIDs[0] != "" || seenIDs[1] != "42" {
		t.Fatalf("connect lastIDs = %v, want [\"\" \"42\"]", seenIDs)
	}
}

func TestStreamCancellationStops(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	connect := func(_ context.Context, _ string) (io.ReadCloser, error) {
		calls++
		if calls == 1 {
			return nopBody("data: a\n\ndata: b\n\n"), nil
		}
		return nil, errors.New("should not reconnect after cancel")
	}

	var data []string
	for ev, err := range sse.Stream(ctx, connect, sse.WithReconnectDelay(0)) {
		if err != nil {
			t.Fatalf("unexpected error (reconnected after cancel?): %v", err)
		}
		data = append(data, ev.Data)
		if ev.Data == "b" {
			cancel()
		}
	}

	if calls != 1 {
		t.Fatalf("connect called %d times, want 1 (no reconnect after cancel)", calls)
	}
	if strings.Join(data, ",") != "a,b" {
		t.Fatalf("data = %v, want [a b]", data)
	}
}

func TestStreamConnectErrorIsTerminal(t *testing.T) {
	t.Parallel()

	boom := errors.New("dial failed")
	connect := func(_ context.Context, _ string) (io.ReadCloser, error) {
		return nil, boom
	}

	var events, errs int
	for _, err := range sse.Stream(context.Background(), connect, sse.WithReconnectDelay(0)) {
		if err != nil {
			if !errors.Is(err, boom) {
				t.Fatalf("err = %v, want boom", err)
			}
			errs++
			break
		}
		events++
	}
	if events != 0 || errs != 1 {
		t.Fatalf("events=%d errs=%d, want 0 and 1", events, errs)
	}
}

type closeRecorder struct {
	io.Reader
	closed bool
}

func (c *closeRecorder) Close() error { c.closed = true; return nil }

func TestStreamConsumerBreakClosesReader(t *testing.T) {
	t.Parallel()

	rec := &closeRecorder{Reader: strings.NewReader("data: a\n\ndata: b\n\n")}
	calls := 0
	connect := func(_ context.Context, _ string) (io.ReadCloser, error) {
		calls++
		return rec, nil
	}

	for ev, err := range sse.Stream(context.Background(), connect, sse.WithReconnectDelay(0)) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_ = ev
		break // stop after the first event
	}

	if !rec.closed {
		t.Fatal("reader was not closed on consumer break")
	}
	if calls != 1 {
		t.Fatalf("connect called %d times, want 1", calls)
	}
}
