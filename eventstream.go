// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package dexpace

import (
	"context"
	"fmt"
	"io"
	"iter"
	"net/http"

	"github.com/dexpace/go-sdk/header"
	"github.com/dexpace/go-sdk/mediatype"
	"github.com/dexpace/go-sdk/sse"
)

// EventStream opens a reconnecting Server-Sent Events stream for req and yields
// decoded events through the client pipeline. Each connection clones req, sets
// Accept: text/event-stream (unless the caller already set Accept) and, after the
// first event id is seen, the Last-Event-ID header, then sends it through Do — so
// auth, logging, tracing, and the rest run per connection.
//
// A non-2xx response or a transport failure on connect ends the stream with that
// error (a connect error is terminal). A mid-stream interruption reconnects
// transparently with the most recent event id. When req carries a body it must be
// replayable (req.GetBody set, as net/http does for in-memory bodies) so reconnects
// can re-send it; SSE is normally a bodyless GET. Cancel the request context to
// stop. The iterator is single-pass. Pass sse.StreamOption values (for example
// sse.WithReconnectDelay) to configure reconnection.
func (c *Client) EventStream(ctx context.Context, req *http.Request, opts ...sse.StreamOption) iter.Seq2[sse.Event, error] {
	connect := func(ctx context.Context, lastEventID string) (io.ReadCloser, error) {
		r := req.Clone(ctx)
		if req.Body != nil && req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, fmt.Errorf("dexpace: rewind event-stream request body: %w", err)
			}
			r.Body = body
		}
		if r.Header.Get(header.Accept) == "" {
			r.Header.Set(header.Accept, mediatype.TextEventStream.Essence())
		}
		if lastEventID != "" {
			r.Header.Set(header.LastEventID, lastEventID)
		}

		resp, err := c.Do(r)
		if err != nil {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			return nil, err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			return nil, fmt.Errorf("dexpace: event stream connect: unexpected status %s", resp.Status)
		}
		return resp.Body, nil
	}
	return sse.Stream(ctx, connect, opts...)
}
