// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package webhook verifies inbound webhook signatures. A [Verifier] checks an
// HMAC-SHA256 signature against a payload in constant time, and VerifyTimestamp
// adds a tolerance window over the common "<unix>.<body>" signed-payload scheme to
// defeat replay.
package webhook
