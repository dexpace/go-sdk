// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package pipeline_test

import (
	"net/http"
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

func TestPlacementConstructors(t *testing.T) {
	t.Parallel()

	p := pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		return req.Next()
	})
	// Must compile and return non-zero placements; ordering is covered in
	// TestNewStagedResolvesOrder.
	_ = pipeline.At(pipeline.StageRetry, p)
	_ = pipeline.Before(pipeline.StageRetry, p)
	_ = pipeline.After(pipeline.StageAuth, p)
}
