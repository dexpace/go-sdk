// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package serde_test

import (
	"testing"

	"github.com/dexpace/go-sdk/serde"
)

func TestJSONRoundTrip(t *testing.T) {
	t.Parallel()

	type payload struct {
		A int    `json:"a"`
		B string `json:"b"`
	}
	in := payload{A: 1, B: "x"}

	data, err := serde.JSON.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != `{"a":1,"b":"x"}` {
		t.Fatalf("Marshal = %s, want {\"a\":1,\"b\":\"x\"}", data)
	}

	var out payload
	if err := serde.JSON.Unmarshal(data, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out != in {
		t.Fatalf("round-trip = %+v, want %+v", out, in)
	}
}

func TestJSONUnmarshalError(t *testing.T) {
	t.Parallel()

	var out struct{ A int }
	if err := serde.JSON.Unmarshal([]byte("{not json"), &out); err == nil {
		t.Fatal("expected an error for invalid JSON")
	}
}
