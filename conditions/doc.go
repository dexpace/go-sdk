// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package conditions provides immutable value types for conditional and range
// requests — entity tags ([ETag]), byte ranges ([Range]), and the precondition
// header set ([Conditions]) — each of which stamps the appropriate headers on an
// *http.Request via its Apply method (or, for ETag, its String form).
package conditions
