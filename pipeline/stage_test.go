// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package pipeline_test

import (
	"testing"

	"github.com/dexpace/go-sdk/pipeline"
)

func TestStagesAreOrdered(t *testing.T) {
	t.Parallel()

	ordered := []pipeline.Stage{
		pipeline.StageClientIdentity,
		pipeline.StageIdempotency,
		pipeline.StageRetry,
		pipeline.StageAuth,
		pipeline.StageDate,
		pipeline.StageLogging,
	}
	for i := 1; i < len(ordered); i++ {
		if !(ordered[i-1] < ordered[i]) {
			t.Fatalf("stage %d not less than stage %d", ordered[i-1], ordered[i])
		}
	}
}
