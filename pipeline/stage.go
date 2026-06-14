// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package pipeline

import "slices"

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

// Placement pairs a [Policy] with where it belongs in stage order. Construct one
// with [At], [Before], or [After] and pass it to [NewStaged].
type Placement struct {
	stage  Stage
	offset int8 // -1 before, 0 at (pillar), +1 after
	policy Policy
}

// At places p exactly at stage s (a "pillar"). When two placements target the
// same stage with the same offset, insertion order is preserved; supplying At
// for a stage already occupied therefore appends after the earlier one.
func At(s Stage, p Policy) Placement { return Placement{stage: s, offset: 0, policy: p} }

// Before places p immediately outside (before) stage s.
func Before(s Stage, p Policy) Placement { return Placement{stage: s, offset: -1, policy: p} }

// After places p immediately after stage s in execution order (one step closer
// to the transport).
func After(s Stage, p Policy) Placement { return Placement{stage: s, offset: 1, policy: p} }

// NewStaged resolves placements into a deterministic order and builds a
// [Pipeline]. Placements are sorted by stage, then by offset (before, at,
// after); placements sharing the same stage and offset keep the order in which
// they were supplied. transport must be non-nil; passing nil panics, matching
// [New].
func NewStaged(transport Transporter, placements ...Placement) Pipeline {
	sorted := make([]Placement, len(placements))
	copy(sorted, placements)
	slices.SortStableFunc(sorted, func(a, b Placement) int {
		return sortKey(a) - sortKey(b)
	})
	policies := make([]Policy, len(sorted))
	for i, pl := range sorted {
		policies[i] = pl.policy
	}
	return New(transport, policies...)
}

func sortKey(p Placement) int { return int(p.stage)*4 + int(p.offset) }
