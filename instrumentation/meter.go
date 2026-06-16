// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package instrumentation

import "context"

// Histogram records a distribution of values (for example request durations in
// seconds).
type Histogram interface {
	Record(ctx context.Context, value float64, attrs ...Attr)
}

// UpDownCounter records an additive value that can rise and fall (for example the
// number of in-flight requests).
type UpDownCounter interface {
	Add(ctx context.Context, delta int64, attrs ...Attr)
}

// Meter creates instruments by name.
type Meter interface {
	Histogram(name string) Histogram
	UpDownCounter(name string) UpDownCounter
}

// NoopMeter is the default Meter; its instruments do nothing.
type NoopMeter struct{}

// Histogram returns a no-op histogram.
func (NoopMeter) Histogram(string) Histogram { return noopHistogram{} }

// UpDownCounter returns a no-op up-down counter.
func (NoopMeter) UpDownCounter(string) UpDownCounter { return noopUpDownCounter{} }

type noopHistogram struct{}

func (noopHistogram) Record(context.Context, float64, ...Attr) {}

type noopUpDownCounter struct{}

func (noopUpDownCounter) Add(context.Context, int64, ...Attr) {}
