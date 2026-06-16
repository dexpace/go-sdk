// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package sse parses a text/event-stream (Server-Sent Events) per the WHATWG
// algorithm, yielding each dispatched [Event] through [Parse] as a range-over-func
// iterator. [Stream] adds a reconnecting consumer over a caller-supplied
// connection, replaying the Last-Event-ID and honoring the server's retry backoff.
package sse
