// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package serde defines a small serialization seam — Marshaler, Unmarshaler, and
// the combined Serde — with a default JSON implementation backed by
// encoding/json. It also provides Tristate, a three-state (absent, null, present)
// value for expressing JSON PATCH payloads with the json ",omitzero" tag.
package serde
