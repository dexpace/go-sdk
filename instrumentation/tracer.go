// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package instrumentation

import "context"

// Attr is a key/value attribute attached to a span or metric.
type Attr struct {
	Key   string
	Value any
}

// SpanContext identifies a span for propagation across process boundaries. The
// zero value means "no active span".
type SpanContext struct {
	TraceID [16]byte
	SpanID  [8]byte
	Sampled bool
}

// IsZero reports whether sc carries no trace or span id.
func (sc SpanContext) IsZero() bool {
	return sc.TraceID == [16]byte{} && sc.SpanID == [8]byte{}
}

// Span is a single unit of work within a trace.
type Span interface {
	// SetAttributes attaches key/value attributes to the span.
	SetAttributes(attrs ...Attr)
	// RecordError records err on the span.
	RecordError(err error)
	// End marks the span complete.
	End()
	// Context returns the span's propagation context.
	Context() SpanContext
}

// Tracer starts spans. StartSpan returns a context carrying the new span so that
// spans started from it become children.
type Tracer interface {
	StartSpan(ctx context.Context, name string) (context.Context, Span)
}

// NoopTracer is the default Tracer; it creates spans that do nothing.
type NoopTracer struct{}

// StartSpan returns ctx unchanged and a no-op span.
func (NoopTracer) StartSpan(ctx context.Context, _ string) (context.Context, Span) {
	return ctx, noopSpan{}
}

type noopSpan struct{}

func (noopSpan) SetAttributes(...Attr) {}
func (noopSpan) RecordError(error)     {}
func (noopSpan) End()                  {}
func (noopSpan) Context() SpanContext  { return SpanContext{} }
