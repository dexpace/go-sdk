// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package sse

import (
	"bufio"
	"io"
	"iter"
	"strconv"
	"strings"
	"time"
)

// maxLineBytes caps a single event-stream line, so a malformed stream cannot grow
// the read buffer without bound. An over-long line surfaces bufio.ErrTooLong.
const maxLineBytes = 1 << 20 // 1 MiB

// Parse interprets r as a text/event-stream and yields each dispatched event,
// following the WHATWG event-stream algorithm. Fields (data, event, id, retry)
// are accumulated and an event is dispatched on a blank line. Lines may end with
// LF or CRLF; comment lines (beginning with ":") are ignored. A read error or an
// over-long line is delivered as the second iteration value, after which
// iteration stops. A partially-accumulated event at end of stream is discarded.
// The iterator is single-pass.
func Parse(r io.Reader) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64<<10), maxLineBytes)

		var data strings.Builder
		eventType := ""
		lastID := ""
		var retry time.Duration
		hasData := false

		for sc.Scan() {
			line := sc.Text()

			if line == "" {
				if hasData {
					payload := strings.TrimSuffix(data.String(), "\n")
					typ := eventType
					if typ == "" {
						typ = "message"
					}
					if !yield(Event{Type: typ, Data: payload, ID: lastID, Retry: retry}, nil) {
						return
					}
				}
				data.Reset()
				eventType = ""
				retry = 0
				hasData = false
				continue
			}

			if line[0] == ':' {
				continue
			}

			field, value, _ := strings.Cut(line, ":")
			value = strings.TrimPrefix(value, " ")
			switch field {
			case "data":
				data.WriteString(value)
				data.WriteByte('\n')
				hasData = true
			case "event":
				eventType = value
			case "id":
				if !strings.ContainsRune(value, '\x00') {
					lastID = value
				}
			case "retry":
				if ms, ok := parseRetry(value); ok {
					retry = ms
				}
			}
		}

		if err := sc.Err(); err != nil {
			yield(Event{}, err)
		}
	}
}

// parseRetry parses an all-ASCII-digits retry value into a duration of
// milliseconds. It reports ok=false for empty or non-numeric input.
func parseRetry(s string) (time.Duration, bool) {
	if s == "" {
		return 0, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return time.Duration(n) * time.Millisecond, true
}
