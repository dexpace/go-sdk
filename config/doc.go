// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package config resolves string-keyed settings from explicit overrides, then the
// process environment, then a caller-supplied default. Typed accessors never fail:
// a missing key or an unparseable value yields the default. Well-known DEXPACE_*
// keys name the settings the umbrella package consults via dexpace.WithConfig.
package config
