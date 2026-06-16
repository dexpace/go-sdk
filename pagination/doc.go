// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package pagination provides a generic, transport-agnostic helper for consuming
// token-paginated API responses as Go range-over-func iterators.
//
// Beyond the token/cursor strategy ([New]), [NewPageNumber] paginates by
// sequential page number and [NewLinkHeader] follows RFC 8288 Link headers
// (parsed by [NextLink]). [WithMaxPages] bounds iteration.
package pagination
