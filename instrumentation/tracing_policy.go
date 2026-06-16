// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package instrumentation

import (
	"fmt"
	"net/http"

	"github.com/dexpace/go-sdk/pipeline"
	"github.com/dexpace/go-sdk/redact"
)

// traceparentHeader is the canonical form of the W3C trace-context header.
const traceparentHeader = "Traceparent"

// NewTracingPolicy returns a pipeline policy that records a span around each
// request it wraps, using tracer (defaulting to NoopTracer) and rendering URLs
// with redactor (defaulting to redact.Default). When the span carries a non-zero
// context and the request has no traceparent header, the policy injects a W3C
// traceparent header so downstream services join the trace.
//
// Granularity is placement-determined: inside the retry policy it records a span
// per attempt; outside it, one span per operation.
func NewTracingPolicy(tracer Tracer, redactor *redact.Redactor) pipeline.Policy {
	if tracer == nil {
		tracer = NoopTracer{}
	}
	if redactor == nil {
		redactor = redact.Default
	}
	return pipeline.PolicyFunc(func(req *pipeline.Request) (*http.Response, error) {
		raw := req.Raw()
		ctx, span := tracer.StartSpan(raw.Context(), raw.Method)
		req.SetContext(ctx)
		defer span.End()

		span.SetAttributes(
			Attr{Key: "http.request.method", Value: raw.Method},
			Attr{Key: "url.full", Value: redactor.URL(raw.URL)},
			Attr{Key: "server.address", Value: hostOf(raw)},
		)
		injectTraceparent(raw, span.Context())

		resp, err := req.Next()
		if err != nil {
			span.RecordError(err)
			return resp, err
		}
		span.SetAttributes(Attr{Key: "http.response.status_code", Value: resp.StatusCode})
		return resp, nil
	})
}

func hostOf(req *http.Request) string {
	if req.URL == nil {
		return ""
	}
	return req.URL.Host
}

// injectTraceparent sets a W3C traceparent header derived from sc when sc is
// non-zero and the request does not already carry one.
func injectTraceparent(req *http.Request, sc SpanContext) {
	if sc.IsZero() || req.Header.Get(traceparentHeader) != "" {
		return
	}
	flags := "00"
	if sc.Sampled {
		flags = "01"
	}
	req.Header.Set(traceparentHeader, fmt.Sprintf("00-%x-%x-%s", sc.TraceID, sc.SpanID, flags))
}
