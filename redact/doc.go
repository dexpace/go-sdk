// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package redact renders log- and trace-safe representations of URLs. It strips
// userinfo and, by default, every query-string value, so secrets carried in a URL
// (API keys, tokens) never reach logs, traces, or error messages. A configurable
// allowlist keeps chosen query-param values visible.
package redact
