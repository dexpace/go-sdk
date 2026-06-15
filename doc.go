// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package dexpace is the entry point for the dexpace Go SDK — an HTTP-client
// toolkit, not an HTTP client.
//
// The SDK provides the request/response plumbing that sits between application
// code and a transport: a composable [pipeline] of policies (retry, auth,
// logging, ...) that runs over the standard library's [net/http] types. It does
// not hide net/http; it builds on it. A [Client] is a thin handle around a
// configured pipeline.
//
// By default Client.Do mirrors http.Client.Do: a non-2xx status is not an error.
// Enable the typed error model with WithErrors to receive a *httperr.ResponseError
// for non-2xx responses and a *httperr.TransportError for transport failures.
//
//	client := dexpace.New(
//		dexpace.WithRetry(retry.Options{MaxRetries: 3}),
//		dexpace.WithCredential(cred),
//	)
//	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example.com/v1/things", nil)
//	resp, err := client.Do(req)
//
// # Layers
//
// The toolkit is layered bottom-up, each layer importable on its own:
//
//   - [github.com/dexpace/go-sdk/pipeline] — the Policy/Transporter contract and
//     the Pipeline that runs an ordered chain of policies.
//   - [github.com/dexpace/go-sdk/transport] — the default net/http-backed
//     Transporter that terminates a pipeline.
//   - [github.com/dexpace/go-sdk/retry], [github.com/dexpace/go-sdk/idempotency],
//     [github.com/dexpace/go-sdk/auth], [github.com/dexpace/go-sdk/logging] —
//     shipped policies (idempotency is default-on for POST).
//   - [github.com/dexpace/go-sdk/httperr] — typed errors for non-success
//     responses.
//   - [github.com/dexpace/go-sdk/mediatype], [github.com/dexpace/go-sdk/header],
//     [github.com/dexpace/go-sdk/pagination] — HTTP value helpers.
//
// All of core depends only on the Go standard library.
package dexpace
