// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package pipeline_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/dexpace/go-sdk/pipeline"
)

func TestStagesAreOrdered(t *testing.T) {
	t.Parallel()

	ordered := []pipeline.Stage{
		pipeline.StageErrors,
		pipeline.StageClientIdentity,
		pipeline.StageIdempotency,
		pipeline.StageRetry,
		pipeline.StageAuth,
		pipeline.StageDate,
		pipeline.StageTracing,
		pipeline.StageMetrics,
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

func TestNewStagedPanicsOnNilTransport(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for nil transport")
		}
	}()
	_ = pipeline.NewStaged(nil)
}

func TestNewStagedResolvesOrder(t *testing.T) {
	t.Parallel()

	var order []string
	mark := func(name string) pipeline.Policy {
		return pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
			order = append(order, name)
			return req.Next()
		})
	}
	tr := transporterFunc(func(req *http.Request) (*http.Response, error) {
		order = append(order, "transport")
		return okResponse(req)
	})

	pl := pipeline.NewStaged(tr,
		pipeline.At(pipeline.StageAuth, mark("auth")),
		pipeline.At(pipeline.StageClientIdentity, mark("ua")),
		pipeline.Before(pipeline.StageRetry, mark("before-retry")),
		pipeline.At(pipeline.StageRetry, mark("retry")),
		pipeline.After(pipeline.StageRetry, mark("after-retry")),
		pipeline.At(pipeline.StageRetry, mark("retry2")),
		pipeline.At(pipeline.StageLogging, mark("log")),
	)

	req, _ := http.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	want := []string{"ua", "before-retry", "retry", "retry2", "after-retry", "auth", "log", "transport"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", order, want)
	}
}
