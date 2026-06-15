// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package idempotency provides a pipeline policy that stamps an Idempotency-Key
// header on requests whose method is not inherently idempotent (POST by
// default), so that the retry policy can safely re-send them. Keys are random
// UUIDv4 values generated from crypto/rand; a caller-supplied key is never
// overwritten.
package idempotency
