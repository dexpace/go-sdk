// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package instrumentation_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dexpace/go-sdk/instrumentation"
)

func TestNoopTracer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	got, span := instrumentation.NoopTracer{}.StartSpan(ctx, "GET")
	if got != ctx {
		t.Fatal("NoopTracer.StartSpan must return the context unchanged")
	}
	if !span.Context().IsZero() {
		t.Fatal("no-op span context must be zero")
	}
	span.SetAttributes(instrumentation.Attr{Key: "k", Value: 1})
	span.RecordError(errors.New("e"))
	span.End()
}

func TestSpanContextIsZero(t *testing.T) {
	t.Parallel()

	var sc instrumentation.SpanContext
	if !sc.IsZero() {
		t.Fatal("zero SpanContext should report IsZero")
	}
	sc.SpanID[0] = 1
	if sc.IsZero() {
		t.Fatal("non-zero SpanContext should not report IsZero")
	}
}
