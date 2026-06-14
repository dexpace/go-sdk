// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package pipeline

// Stage names an anchor point in the standard policy order, from outermost
// (StageClientIdentity) to innermost (StageLogging). Stages are used at assembly
// time to place policies deterministically; the running pipeline is still a flat
// ordered list. Use [At], [Before], and [After] to position a [Policy] relative
// to a Stage, then build with [NewStaged].
type Stage int

// The standard stages, in execution order. An earlier stage wraps the later
// ones: retry (StageRetry) is outside auth (StageAuth), so a 401-triggered token
// refresh re-runs per attempt; logging (StageLogging) is innermost, so it records
// the request as actually sent.
const (
	StageClientIdentity Stage = iota + 1 // user-agent and similar identity headers
	StageIdempotency                     // idempotency-key, minted once outside retry
	StageRetry                           // retry pillar; wraps everything below
	StageAuth                            // credential stamping / refresh
	StageDate                            // Date header
	StageLogging                         // innermost; logs the on-the-wire request
)
