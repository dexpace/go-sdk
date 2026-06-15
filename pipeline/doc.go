// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package pipeline defines the request-processing contract at the heart of the
// SDK: a [Pipeline] runs an ordered chain of [Policy] values over an
// *http.Request and terminates in a [Transporter] that performs the network
// round-trip.
//
// A policy receives a [Request], may inspect or mutate the underlying
// *http.Request, and then calls [Request.Next] to invoke the rest of the chain.
// Returning without calling Next short-circuits the pipeline — useful for cache
// hits or fast-fail validation. Because Next can be called more than once, a
// policy can implement retries by rewinding the body with [Request.RewindBody]
// between attempts.
//
// The design intentionally leans on the standard library: requests and responses
// are net/http types, and *http.Client satisfies [Transporter] through the thin
// adapter in package transport. This package adds the composition seam, not a
// replacement for net/http.
//
// # Stage ordering
//
// Most callers build a pipeline through the umbrella package's options, but the
// ordering is expressed here. A [Stage] names an anchor point in the standard
// policy order; [At], [Before], and [After] position a [Policy] relative to a
// stage; [NewStaged] resolves those placements into the flat ordered list that
// [New] runs. Resolution is stable: placements are ordered by stage, then by
// before/at/after, and ties keep insertion order. Placing two policies At the
// same stage runs them in insertion order (the second effectively "after" the
// first), which is how a built-in pillar is supplemented or replaced.
package pipeline
