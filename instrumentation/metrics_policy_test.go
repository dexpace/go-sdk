// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package instrumentation_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/dexpace/go-sdk/instrumentation"
	"github.com/dexpace/go-sdk/pipeline"
)

type recordCall struct {
	value float64
	attrs []instrumentation.Attr
}

type fakeHistogram struct{ calls []recordCall }

func (h *fakeHistogram) Record(_ context.Context, v float64, a ...instrumentation.Attr) {
	h.calls = append(h.calls, recordCall{v, a})
}

type fakeUpDown struct {
	sum   int64
	calls int
}

func (c *fakeUpDown) Add(_ context.Context, d int64, _ ...instrumentation.Attr) {
	c.sum += d
	c.calls++
}

type fakeMeter struct {
	hist *fakeHistogram
	ud   *fakeUpDown
}

func (m *fakeMeter) Histogram(string) instrumentation.Histogram         { return m.hist }
func (m *fakeMeter) UpDownCounter(string) instrumentation.UpDownCounter { return m.ud }

func TestMetricsPolicyRecordsDurationAndBalancesInflight(t *testing.T) {
	t.Parallel()

	m := &fakeMeter{hist: &fakeHistogram{}, ud: &fakeUpDown{}}
	pl := pipeline.New(transporterFunc(okResp), instrumentation.NewMetricsPolicy(m))

	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/x", nil)
	resp, err := pl.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if len(m.hist.calls) != 1 {
		t.Fatalf("histogram calls = %d, want 1", len(m.hist.calls))
	}
	if !hasAttrKey(m.hist.calls[0].attrs, "http.response.status_code") {
		t.Fatalf("duration attrs missing status_code: %v", m.hist.calls[0].attrs)
	}
	if m.ud.calls != 2 || m.ud.sum != 0 {
		t.Fatalf("in-flight gauge calls=%d sum=%d, want 2 and 0 (balanced)", m.ud.calls, m.ud.sum)
	}
}

func TestMetricsPolicyBalancesInflightOnError(t *testing.T) {
	t.Parallel()

	m := &fakeMeter{hist: &fakeHistogram{}, ud: &fakeUpDown{}}
	pl := pipeline.New(transporterFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("boom")
	}), instrumentation.NewMetricsPolicy(m))

	req, _ := http.NewRequest(http.MethodGet, "https://api.example.test/x", nil)
	_, _ = pl.Do(req)

	if m.ud.calls != 2 || m.ud.sum != 0 {
		t.Fatalf("in-flight gauge calls=%d sum=%d, want balanced on error", m.ud.calls, m.ud.sum)
	}
	if len(m.hist.calls) != 1 || !hasAttr(m.hist.calls[0].attrs, "error", true) {
		t.Fatalf("duration on error missing error attr: %v", m.hist.calls)
	}
}
