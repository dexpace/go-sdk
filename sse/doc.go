// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package sse parses a text/event-stream (Server-Sent Events) per the WHATWG
// algorithm, yielding each dispatched [Event] through [Parse] as a range-over-func
// iterator. It operates on any io.Reader; a reconnecting connection
// (Last-Event-ID replay, server retry backoff) is intentionally left to the
// caller and a future addition.
package sse
