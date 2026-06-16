// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package instrumentation

import (
	"net/http"
	"time"

	"github.com/dexpace/go-sdk/pipeline"
)

const (
	metricRequestDuration = "http.client.request.duration"
	metricActiveRequests  = "http.client.active_requests"
)

// NewMetricsPolicy returns a pipeline policy that records request metrics using
// meter (defaulting to NoopMeter): a request-duration histogram (seconds) and an
// active-requests up-down counter, each tagged with the request method and, on
// success, the response status code.
//
// Granularity is placement-determined, like the tracing policy.
func NewMetricsPolicy(meter Meter) pipeline.Policy {
	if meter == nil {
		meter = NoopMeter{}
	}
	duration := meter.Histogram(metricRequestDuration)
	active := meter.UpDownCounter(metricActiveRequests)

	return pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		raw := req.Raw()
		ctx := raw.Context()
		method := Attr{Key: "http.request.method", Value: raw.Method}

		active.Add(ctx, 1, method)
		defer active.Add(ctx, -1, method)

		start := time.Now()
		resp, err := req.Next()
		elapsed := time.Since(start).Seconds()

		if err != nil {
			duration.Record(ctx, elapsed, method, Attr{Key: "error", Value: true})
			return resp, err
		}
		duration.Record(ctx, elapsed, method, Attr{Key: "http.response.status_code", Value: resp.StatusCode})
		return resp, nil
	})
}
