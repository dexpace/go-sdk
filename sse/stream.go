// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package sse

import (
	"context"
	"io"
	"iter"
	"time"
)

// defaultReconnectDelay is the wait between reconnects unless overridden by an
// option or a server retry value.
const defaultReconnectDelay = 3 * time.Second

// ConnectFunc opens a fresh event-stream connection. It receives the most recent
// event id (empty on the first connect) so the server can resume via the
// Last-Event-ID request header. Stream closes the returned reader when the
// connection ends.
type ConnectFunc func(ctx context.Context, lastEventID string) (io.ReadCloser, error)

// StreamOption configures [Stream].
type StreamOption func(*streamConfig)

type streamConfig struct {
	delay time.Duration
	wait  func(ctx context.Context, d time.Duration) bool
}

// WithReconnectDelay sets the wait between reconnects. It defaults to three
// seconds and is overridden by any server-sent retry value. A delay <= 0
// reconnects immediately.
func WithReconnectDelay(d time.Duration) StreamOption {
	return func(c *streamConfig) { c.delay = d }
}

// withWait injects the reconnect-wait function. Test seam.
func withWait(fn func(ctx context.Context, d time.Duration) bool) StreamOption {
	return func(c *streamConfig) { c.wait = fn }
}

// Stream yields events from a reconnecting SSE source. It calls connect, parses
// events until the stream ends (EOF or a read error), waits the reconnection
// delay, then reconnects with the most recent event id. A connect error is
// delivered as the iterator error and ends the stream; cancel ctx to stop. The
// iterator is single-pass.
func Stream(ctx context.Context, connect ConnectFunc, opts ...StreamOption) iter.Seq2[Event, error] {
	cfg := streamConfig{delay: defaultReconnectDelay, wait: realWait}
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(yield func(Event, error) bool) {
		lastID := ""
		delay := cfg.delay
		for {
			if ctx.Err() != nil {
				return
			}
			rc, err := connect(ctx, lastID)
			if err != nil {
				yield(Event{}, err)
				return
			}

			stopped := false
			for ev, perr := range Parse(rc) {
				if perr != nil {
					break // mid-stream read error: reconnect transparently
				}
				if ev.ID != "" {
					lastID = ev.ID
				}
				if ev.Retry > 0 {
					delay = ev.Retry
				}
				if !yield(ev, nil) {
					stopped = true
					break
				}
			}
			_ = rc.Close()

			if stopped {
				return
			}
			if !cfg.wait(ctx, delay) {
				return
			}
		}
	}
}

// realWait waits for d, returning false if ctx is (or becomes) done. A
// non-positive d returns true immediately, after the context check.
func realWait(ctx context.Context, d time.Duration) bool {
	if ctx.Err() != nil {
		return false
	}
	if d <= 0 {
		return true
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
