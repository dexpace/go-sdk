// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package dexpace_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/dexpace/go-sdk"
	"github.com/dexpace/go-sdk/retry"
)

// ExampleClient_Do shows the smallest end-to-end use: build a client, send a
// standard *http.Request, read the response.
func ExampleClient_Do() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "pong")
	}))
	defer srv.Close()

	client := dexpace.New(
		dexpace.WithRetry(retry.Options{MaxRetries: 2}),
	)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		panic(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	fmt.Println(string(body))
	// Output: pong
}
