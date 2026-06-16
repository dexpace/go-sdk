# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-06-16

First release of the dexpace Go SDK: a transport-agnostic HTTP-client toolkit
built on `net/http`, with zero third-party runtime dependencies. Requests and
responses are standard `*http.Request` / `*http.Response` values, and the
transport seam is satisfied by `*http.Client`.

### Added

#### Pipeline and client

- A composable policy pipeline (`pipeline`) that runs an ordered chain of
  policies over an `*http.Request` and terminates in a transport. Policies can
  inspect or mutate a request, continue the chain, or short-circuit, and can
  replay the request body across attempts.
- A default `net/http`-backed transport (`transport`) that terminates a
  pipeline, cloned from `http.DefaultTransport` with larger idle-connection
  limits.
- An umbrella `Client` configured through functional options, wiring the default
  policy stack with sensible ordering.

#### Resilience

- Retry policy (`retry`) with exponential backoff and full jitter, support for
  the `Retry-After` header, and automatic request-body rewind so retried
  requests resend their payload.
- Idempotency-key stamping (`idempotency`) that adds an `Idempotency-Key` header
  to POST requests by default, and can be disabled per client.

#### Authentication

All credential policies require an HTTPS transport and refuse to attach
credentials over plaintext.

- Bearer-token authentication (`auth`) driven by a pluggable `TokenCredential`,
  with a token cache that can be shared across clients.
- HTTP Basic authentication.
- API-key authentication via a configurable header.
- HTTP Digest authentication (RFC 7616, MD5/SHA-256, `qop=auth`).

#### Errors, logging, and observability

- An opt-in typed error model (`httperr`): a `ResponseError` for non-success
  responses (buffering and rewinding the response body) and a `TransportError`
  for transport failures.
- Structured request/response logging (`logging`) via `log/slog`.
- Vendor-neutral tracing and metrics SPIs (`instrumentation`) with no-op
  defaults, plus tracing and metrics policies. The tracing policy emits a span
  per request and injects a W3C `traceparent` header; the metrics policy records
  request duration and in-flight requests.
- Default-deny URL redaction (`redact`) shared by logs, traces, and errors:
  userinfo is stripped and query values are redacted unless explicitly
  allowlisted.

#### Value types and helpers

- Immutable media-type value (`mediatype`) with parsing and common constants.
- Canonical HTTP header-name constants (`header`).
- Conditional- and range-request value types (`conditions`): ETag, Range, and
  Conditions.
- A serialization seam (`serde`) with a JSON default and a `Tristate` type for
  distinguishing absent, null, and present fields in PATCH payloads.
- A layered settings resolver (`config`) that sources values from explicit
  overrides, then `DEXPACE_*` environment variables, then defaults.

#### Streaming and bodies

- Server-Sent Events (`sse`): a WHATWG-compliant `text/event-stream` parser, a
  reconnecting stream that replays `Last-Event-ID` after an interruption, and
  `Client.EventStream` to run a stream through the pipeline.
- JSON Lines / NDJSON streaming decoder (`jsonl`) exposed as a generic
  `iter.Seq2`.
- Multipart `multipart/form-data` request-body builder (`formdata`) with
  replayable bodies and file uploads.
- Generic pagination (`pagination`) as `iter.Seq2` range-over-func iterators,
  with cursor/token, page-number, and RFC 8288 Link-header strategies and a
  page cap.

#### Webhooks

- Inbound webhook signature verification (`webhook`): constant-time HMAC-SHA256
  comparison with a configurable timestamp-tolerance window for replay
  protection.

### Requirements

- Go 1.26 or newer.
- Zero third-party runtime dependencies; only the standard library is imported
  by non-test code.

[Unreleased]: https://github.com/dexpace/go-sdk/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/dexpace/go-sdk/releases/tag/v0.1.0
