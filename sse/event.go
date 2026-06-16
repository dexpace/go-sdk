// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package sse

import "time"

// Event is one Server-Sent Event dispatched by [Parse].
type Event struct {
	// Type is the event type, or "message" when the stream did not specify one.
	Type string
	// Data is the event payload: multiple data lines joined with "\n", with the
	// trailing newline removed.
	Data string
	// ID is the most recent event id. It is sticky: an event without an id field
	// keeps the previous value, per the WHATWG specification.
	ID string
	// Retry is the reconnection-time hint from a retry field on this event, or 0
	// when none was given.
	Retry time.Duration
}
