<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/dexpace-wordmark-dark.svg">
    <img alt="dexpace" src="docs/assets/dexpace-wordmark-light.svg" width="320">
  </picture>
</p>

<h1 align="center">Go SDKs Platform</h1>

The Go counterpart to [`dexpace/java-sdk`](https://github.com/dexpace/java-sdk)
and [`dexpace/python-sdk`](https://github.com/dexpace/python-sdk). It is an
**HTTP-client toolkit, not an HTTP client**: it supplies the request/response
plumbing — a composable pipeline of policies (retry, auth, logging, …) — that
sits between application code and a transport.

The Go port leans on the standard library. Requests and responses are
`net/http` types, the transport seam is satisfied by `*http.Client`, and bodies
are plain `io.Reader`/`io.ReadCloser`. There is no reinvented HTTP model and no
Okio-style I/O layer; the toolkit adds the composition seam on top of
`net/http`, the way [`azcore`](https://pkg.go.dev/github.com/Azure/azure-sdk-for-go/sdk/azcore)
and the AWS SDK for Go do.

```go
client := dexpace.New(
    dexpace.WithRetry(retry.Options{MaxRetries: 3}),
    dexpace.WithCredential(cred, "https://api.example.com/.default"),
    dexpace.WithLogging(nil), // nil → slog.Default()
)

req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example.com/v1/things", nil)
resp, err := client.Do(req)
if err != nil {
    return err
}
defer resp.Body.Close()
if rerr := httperr.FromResponse(resp); rerr != nil {
    return rerr // *httperr.ResponseError for any 4xx/5xx
}
```

## Architecture

Layered bottom-up; each layer is importable on its own and depends only on the
standard library.

| Package | Responsibility |
|---|---|
| [`pipeline`](./pipeline) | The `Policy` / `Transporter` contract and the `Pipeline` that runs an ordered policy chain over an `*http.Request`. `Request.Next` advances the chain; `Request.RewindBody` replays the body for retries. |
| [`transport`](./transport) | The default `net/http`-backed `Transporter` that terminates a pipeline. |
| [`retry`](./retry) | Retry policy — exponential backoff with full jitter, `Retry-After`, body rewind. |
| [`idempotency`](./idempotency) | Idempotency-key policy (default-on for POST). |
| [`auth`](./auth) | `TokenCredential` contract and `BearerTokenPolicy` (HTTPS-only, cached). |
| [`logging`](./logging) | Structured request/response logging via `log/slog`, with URL redaction. |
| [`instrumentation`](./instrumentation) | Vendor-neutral `Tracer`/`Meter` SPIs with no-op defaults, plus tracing and metrics policies. |
| [`redact`](./redact) | Default-deny URL redactor (strips userinfo, redacts query values) shared by logs, traces, and errors. |
| [`httperr`](./httperr) | `ResponseError` for non-success responses; buffers and rewinds the body. |
| [`mediatype`](./mediatype) | Immutable media-type value with parsing and common constants. |
| [`header`](./header) | Canonical HTTP header-name constants. |
| [`pagination`](./pagination) | Generic pagination as `iter.Seq2` range-over-func iterators — cursor/token, page-number, and RFC 8288 Link-header strategies, with a `WithMaxPages` cap. |
| [`conditions`](./conditions) | Conditional- and range-request value types (ETag, Range, Conditions). |
| [`config`](./config) | Layered override → environment → default settings resolver; non-failing typed getters. |
| [`serde`](./serde) | Serialization seam (Marshaler/Unmarshaler) with a JSON default, plus Tristate for PATCH payloads. |
| [`sse`](./sse) | Server-Sent Events (text/event-stream) WHATWG parser + reconnecting Stream (Last-Event-ID replay). |
| [`jsonl`](./jsonl) | JSON Lines / NDJSON streaming decoder (`iter.Seq2`). |
| [`webhook`](./webhook) | Inbound webhook signature verification (constant-time HMAC + timestamp tolerance). |
| [`formdata`](./formdata) | Multipart/form-data request body builder (replayable; file uploads). |
| root [`dexpace`](.) | Umbrella `Client` wiring the default policy stack. |

### Pipeline order

`dexpace.New` assembles policies outermost-first:

```
[errors] → client-identity → idempotency → retry → auth → [date] → [tracing] → [metrics] → logging → custom → transport
```

Retry wraps the inner policies, so auth re-runs (and may refresh its token) on
every attempt and logging records each attempt. Build a custom order directly
with `pipeline.New(transport, policies...)` when you need something else.

An `Idempotency-Key` is sent on POST requests by default (disable with
`WithoutIdempotency`); `WithDate` is opt-in.

By default `Client.Do` follows `net/http` semantics, where a non-2xx status is
not an error. `WithErrors` opts into the typed error model: `Client.Do` then
returns a `*httperr.ResponseError` for non-2xx responses and a
`*httperr.TransportError` for transport failures. It is off by default.

### Observability

Tracing and metrics are opt-in and route through the `instrumentation`
package's vendor-neutral SPIs (no-op by default, so nothing is emitted until you
wire a backend):

- `WithTracing(tracer)` — installs a tracing policy that emits a span per request
  via the instrumentation `Tracer` SPI and injects a W3C `traceparent` header.
- `WithMetrics(meter)` — installs a metrics policy recording request duration and
  in-flight requests via the instrumentation `Meter` SPI.
- `WithRedactionAllowlist(params...)` — preserves the listed query-param values in
  redacted URLs (logs and traces); all other query values are redacted by default.

URLs are redacted by default across logs, traces, and errors: userinfo is
stripped and query values are redacted unless allowlisted with
`WithRedactionAllowlist`.

### Authentication and configuration

- `WithCredential(cred, scopes...)` — authenticates requests with bearer tokens
  from a `TokenCredential` (HTTPS-only, cached).
- `WithTokenCache(cache)` — shares a bearer-token cache (an `auth.TokenCache`, in-memory
  by default) across clients so a cached token is reused.
- `WithBasicAuth(username, password)` — authenticates requests with HTTP Basic auth (HTTPS-only).
- `WithAPIKey(header, key)` — sets an API-key header on every request (HTTPS-only).
- `WithConfig(cfg)` — sources defaults from `DEXPACE_*` environment variables —
  `DEXPACE_USER_AGENT`, `DEXPACE_MAX_RETRIES` (0 or negative disables retries),
  `DEXPACE_RETRY_BASE_DELAY`, `DEXPACE_HTTP_TIMEOUT` (default transport only) — for
  settings not set explicitly; explicit options always win.

## Requirements

Go **1.26+**. The module targets modern idioms: generics, range-over-func
iterators (`iter.Seq2`), `math/rand/v2`, `log/slog`, and the `min`/`max`
builtins.

## Development

```bash
make check   # tidy + fmt + vet + lint + test (race + coverage)
make test    # go test -race -covermode=atomic ./...
make lint    # golangci-lint
```

The SDK ships **zero third-party runtime dependencies**; only the standard
library is imported by non-test code. See [`CONTRIBUTING.md`](./CONTRIBUTING.md)
for conventions and [`CLAUDE.md`](./CLAUDE.md) for the enforced rules.

## License

MIT — see [LICENSE](./LICENSE).
