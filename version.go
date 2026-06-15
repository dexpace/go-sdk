// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package dexpace

// Version is the semantic version of the SDK. It is reported in the default
// User-Agent header so servers and proxies can identify the client.
const Version = "0.1.0"

// userAgent is the default User-Agent value applied by [New] unless overridden.
const userAgent = "dexpace-go-sdk/" + Version
