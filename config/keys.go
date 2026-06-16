// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package config

// Well-known environment-variable keys the SDK consults. Callers may use any key
// with the getters; these are the ones dexpace.WithConfig reads.
const (
	// EnvMaxRetries is the retry count after the initial attempt (int). A value of
	// 0 or negative disables retries; absent leaves the SDK default in place.
	EnvMaxRetries = "DEXPACE_MAX_RETRIES"
	// EnvRetryBaseDelay is the first retry backoff interval (duration, e.g. "800ms").
	EnvRetryBaseDelay = "DEXPACE_RETRY_BASE_DELAY"
	// EnvHTTPTimeout is the per-request timeout for the default transport
	// (duration, e.g. "30s").
	EnvHTTPTimeout = "DEXPACE_HTTP_TIMEOUT"
	// EnvUserAgent overrides the default User-Agent (string).
	EnvUserAgent = "DEXPACE_USER_AGENT"
)
