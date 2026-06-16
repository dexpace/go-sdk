// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package dexpace is the entry point for the dexpace Go SDK — an HTTP-client
// toolkit, not an HTTP client.
//
// The SDK provides the request/response plumbing that sits between application
// code and a transport: a composable [pipeline] of policies (retry, auth,
// logging, ...) that runs over the standard library's [net/http] types. It does
// not hide net/http; it builds on it. A [Client] is a thin handle around a
// configured pipeline.
//
// By default Client.Do mirrors http.Client.Do: a non-2xx status is not an error.
// Enable the typed error model with WithErrors to receive a *httperr.ResponseError
// for non-2xx responses and a *httperr.TransportError for transport failures.
//
// Tracing and metrics are opt-in: WithTracing and WithMetrics install policies
// that emit spans and request metrics through the instrumentation package's
// vendor-neutral interfaces (no-op by default). WithRedactionAllowlist controls
// which query-param values survive redaction in logs and traces.
//
// Beyond bearer tokens (WithCredential), WithBasicAuth and WithAPIKey authenticate
// requests with HTTP Basic auth or an API-key header, and WithDigestAuth performs
// HTTP Digest Access Authentication (RFC 7616); all require HTTPS.
//
// WithTokenCache shares a bearer-token cache across clients (auth.TokenCache, with
// an in-memory default).
//
// WithConfig sources client defaults (User-Agent, retry settings, transport
// timeout) from the environment via the config package, for any setting not set
// explicitly.
//
//	client := dexpace.New(
//		dexpace.WithRetry(retry.Options{MaxRetries: 3}),
//		dexpace.WithCredential(cred),
//	)
//	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example.com/v1/things", nil)
//	resp, err := client.Do(req)
//
// # Layers
//
// The toolkit is layered bottom-up, each layer importable on its own:
//
//   - [github.com/dexpace/go-sdk/pipeline] — the Policy/Transporter contract and
//     the Pipeline that runs an ordered chain of policies.
//   - [github.com/dexpace/go-sdk/transport] — the default net/http-backed
//     Transporter that terminates a pipeline.
//   - [github.com/dexpace/go-sdk/retry], [github.com/dexpace/go-sdk/idempotency],
//     [github.com/dexpace/go-sdk/auth], [github.com/dexpace/go-sdk/logging] —
//     shipped policies (idempotency is default-on for POST).
//   - [github.com/dexpace/go-sdk/httperr] — typed errors for non-success
//     responses.
//   - [github.com/dexpace/go-sdk/mediatype], [github.com/dexpace/go-sdk/header],
//     [github.com/dexpace/go-sdk/pagination] — HTTP value helpers.
//
// The conditions package provides value types for conditional and range requests
// (ETag, Range, Conditions) that stamp the appropriate headers on a request.
//
// The serde package provides a serialization seam (Marshaler/Unmarshaler with a
// JSON default) and Tristate for JSON PATCH payloads; httperr.ResponseError.DecodeInto
// decodes an error body into a typed value.
//
// The jsonl package decodes streaming JSON Lines (NDJSON) responses into typed
// values.
//
// The sse package parses Server-Sent Events (text/event-stream) into a
// range-over-func iterator of events, with a reconnecting Stream that replays the
// Last-Event-ID.
//
// The webhook package verifies inbound webhook signatures (constant-time HMAC
// with a timestamp-tolerance window).
//
// The formdata package builds replayable multipart/form-data request bodies for
// file uploads.
//
// All of core depends only on the Go standard library.
package dexpace
