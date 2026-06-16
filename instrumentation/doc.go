// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package instrumentation defines vendor-neutral tracing and metrics seams — a
// Tracer/Span and a Meter with Histogram and UpDownCounter instruments — together
// with no-op defaults and the pipeline policies that drive them. The SDK emits
// spans and metrics through these interfaces without depending on any specific
// observability backend; adapters to OpenTelemetry or similar live in user code.
package instrumentation
