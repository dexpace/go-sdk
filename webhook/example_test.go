// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package webhook_test

import (
	"fmt"

	"github.com/dexpace/go-sdk/webhook"
)

func ExampleVerifier_Verify() {
	secret := []byte("shared-secret")
	payload := []byte(`{"event":"ping"}`)

	// The sender computes the signature over the payload with the shared secret.
	signature := webhook.Sign(secret, payload)

	// The receiver verifies the payload against the signature.
	verifier := webhook.NewVerifier(secret)
	err := verifier.Verify(payload, signature)

	fmt.Println(err == nil)
	// Output:
	// true
}
