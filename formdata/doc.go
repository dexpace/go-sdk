// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package formdata builds multipart/form-data request bodies. A [Form] collects
// text fields and file parts and produces a replayable body together with the
// matching Content-Type (including the boundary), so the body survives retries.
// Use [Form.NewRequest] for a ready-to-send *http.Request.
package formdata
