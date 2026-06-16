// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package jsonl decodes a stream of JSON values (JSON Lines / NDJSON) into typed
// values via a range-over-func iterator. Values may be separated by any JSON
// whitespace, so newline-delimited streams and concatenated values both work.
package jsonl
