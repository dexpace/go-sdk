// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package auth provides authentication primitives for the SDK: the
// [TokenCredential] contract that supplies bearer tokens, and the
// [BearerTokenPolicy] that attaches them to outgoing requests.
//
// Credentials are decoupled from policies so an application can implement
// [TokenCredential] however it likes — static token, client-credentials grant,
// workload identity — and reuse the shipped policy unchanged.
package auth
