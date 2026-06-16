# Config — design

**Date:** 2026-06-16
**Status:** Approved (standing delegation); ready for implementation planning
**Subsystem:** #4 of the Go SDK platform-parity roadmap

## Context

The `config` package is a placeholder. Java and Python expose a layered
`Configuration` (explicit override → environment → default) with typed accessors
that never throw on a parse failure, and a duration parser. This subsystem brings
that to Go idiomatically and wires a thin, opt-in integration into the umbrella
so environment variables can supply client defaults.

## Decisions

1. **Layered resolution:** explicit overrides → environment (`os.LookupEnv`) →
   caller-supplied default. The lookup key *is* the environment-variable name.
2. **Typed getters never fail:** `GetString/GetInt/GetBool/GetDuration` return the
   supplied default on a missing key or a parse error — never panic, never return
   an error (matching Java/Python).
3. **Durations** use `time.ParseDuration` (Go shorthand: `5s`, `2m30s`, `1h`).
   ISO-8601 (`PT5S`) is out of scope.
4. **No builder.** Construct with `config.New(opts...)` and functional options.
5. **Proxy is not handled** — `net/http` already honors
   `HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY` via the default transport.
6. **Umbrella integration is opt-in and order-independent:** `dexpace.WithConfig(cfg)`
   supplies defaults only for fields the caller did not explicitly set.

## Architecture

### `config` package (stdlib-only)

```go
// Config resolves string-keyed values from explicit overrides, then the process
// environment, then a caller-supplied default. It is safe for concurrent use.
type Config struct {
	overrides map[string]string
}

type Option func(*Config)

// New builds a Config from the given options.
func New(opts ...Option) *Config

// WithOverride sets a single override that takes precedence over the environment.
func WithOverride(key, value string) Option

// WithOverrides sets several overrides at once (copied defensively).
func WithOverrides(m map[string]string) Option

// Lookup returns the raw value for key (override first, then environment) and
// whether it was found.
func (c *Config) Lookup(key string) (string, bool)

// GetString returns the value for key, or def when unset.
func (c *Config) GetString(key, def string) string

// GetInt returns the value parsed as an int, or def when unset or unparseable.
func (c *Config) GetInt(key string, def int) int

// GetBool returns the value parsed as a bool (strconv.ParseBool), or def when
// unset or unparseable.
func (c *Config) GetBool(key string, def bool) bool

// GetDuration returns the value parsed with time.ParseDuration, or def when unset
// or unparseable.
func (c *Config) GetDuration(key string, def time.Duration) time.Duration
```

Resolution detail: `Lookup` checks `overrides[key]` first; if absent, falls back
to `os.LookupEnv(key)`. `WithOverrides` copies the map so later caller mutation
cannot affect the `Config`. The zero `*Config` is not used directly; `New()` with
no options is valid (env-only).

### Well-known keys (`config` package constants)

```go
const (
	EnvMaxRetries     = "DEXPACE_MAX_RETRIES"      // int
	EnvRetryBaseDelay = "DEXPACE_RETRY_BASE_DELAY" // duration
	EnvHTTPTimeout    = "DEXPACE_HTTP_TIMEOUT"     // duration
	EnvUserAgent      = "DEXPACE_USER_AGENT"       // string
)
```

These are the keys the umbrella integration consults; callers may use any keys
with the getters.

### Umbrella integration (`dexpace.WithConfig`)

```go
// WithConfig supplies client defaults from cfg for any setting the caller did not
// set explicitly: max retries and base delay (DEXPACE_MAX_RETRIES,
// DEXPACE_RETRY_BASE_DELAY), the default-transport timeout (DEXPACE_HTTP_TIMEOUT),
// and the User-Agent (DEXPACE_USER_AGENT). Explicit options always win,
// regardless of option order.
func WithConfig(cfg *config.Config) Option
```

Implementation: `WithConfig` stores the `*config.Config` on the umbrella config.
After all options are applied, `New` fills only **unset** fields from it:

- User-Agent: when `cfg.userAgent == ""`, use `cfg.source.GetString(EnvUserAgent, version)`.
- Retry: when `cfg.retry == nil`, build `retry.Options{MaxRetries: GetInt(EnvMaxRetries, 0), BaseDelay: GetDuration(EnvRetryBaseDelay, 0)}` (zero selects the retry package's own defaults, so an unset env var changes nothing).
- Timeout: when `cfg.transport == nil`, build the default transport with
  `transport.WithTimeout(GetDuration(EnvHTTPTimeout, 0))` (zero timeout = no
  timeout, the current default).

Because the fill happens after option application and only for unset fields, it is
order-independent: passing `WithConfig` before or after `WithRetry` yields the
same result (explicit `WithRetry` wins).

## Edge cases

- Missing env var / empty override value → getters return the default. (An empty
  string override is treated as "set to empty" by `Lookup` but yields the default
  for typed getters that fail to parse `""`.)
- `GetInt`/`GetDuration`/`GetBool` on an unparseable value → default (no error).
- `WithConfig(nil)` → no-op (no source, defaults unchanged).
- `WithOverrides` copies its input map; a nil map is fine.
- Negative `DEXPACE_MAX_RETRIES` is passed through to retry (which treats a
  negative value as "disable retries", its documented behavior).

## Package layout

| Path | Change |
|---|---|
| `config/doc.go` | replace placeholder comment |
| `config/config.go` (+ test) | `Config`, `New`, options, getters |
| `config/keys.go` | well-known `Env*` key constants |
| `options.go` (+ test) | `WithConfig` + source field |
| `client.go` | fill unset fields from the config source |
| `doc.go`, `README.md` | document |

## Testing

- Getters: override beats env; env used when no override; default when unset;
  parse-error → default for int/bool/duration; `time.ParseDuration` shorthand
  (`2m30s`) parsed correctly; `GetBool` via `strconv.ParseBool` (`true`/`1`/`0`).
- `WithOverrides` map is copied (mutating the caller's map afterward does not
  change results).
- Env-based tests use `t.Setenv` (which forbids `t.Parallel` in that test — those
  specific env tests run without `t.Parallel`; pure override/default tests stay
  parallel).
- Umbrella: `WithConfig` sets user-agent / retry / timeout from env when unset;
  explicit options override regardless of order; `WithConfig(nil)` is a no-op.
- No third-party dependencies; `gofmt`/`go vet`/`go test -race` clean.

## Out of scope (deferred)

- ISO-8601 duration parsing.
- Proxy configuration (net/http handles it).
- System-property-style layering (no Go analogue).
- A config-file loader (env + overrides only).
- Base-endpoint/URL configuration (the SDK is transport-agnostic; callers build
  their own request URLs).
