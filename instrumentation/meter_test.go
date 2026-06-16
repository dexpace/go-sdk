// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package instrumentation_test

import (
	"context"
	"testing"

	"github.com/dexpace/go-sdk/instrumentation"
)

func TestNoopMeter(t *testing.T) {
	t.Parallel()

	m := instrumentation.NoopMeter{}
	m.Histogram("h").Record(context.Background(), 1.5, instrumentation.Attr{Key: "k", Value: "v"})
	m.UpDownCounter("c").Add(context.Background(), 1)
	m.UpDownCounter("c").Add(context.Background(), -1)
}
