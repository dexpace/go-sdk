// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package sse

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestStreamRetryOverridesDelay(t *testing.T) {
	var recorded []time.Duration
	wait := func(ctx context.Context, d time.Duration) bool {
		recorded = append(recorded, d)
		return ctx.Err() == nil
	}

	calls := 0
	connect := func(_ context.Context, _ string) (io.ReadCloser, error) {
		calls++
		if calls == 1 {
			return io.NopCloser(strings.NewReader("retry: 2000\ndata: a\n\n")), nil
		}
		return nil, errors.New("stop")
	}

	var gotErr error
	for _, err := range Stream(context.Background(), connect,
		WithReconnectDelay(time.Hour), withWait(wait)) {
		if err != nil {
			gotErr = err
			break
		}
	}

	if gotErr == nil {
		t.Fatal("expected the stop error")
	}
	if len(recorded) != 1 || recorded[0] != 2000*time.Millisecond {
		t.Fatalf("recorded wait delays = %v, want [2s] (retry overrode the hour default)", recorded)
	}
}
