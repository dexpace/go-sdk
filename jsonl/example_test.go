// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package jsonl_test

import (
	"fmt"
	"strings"

	"github.com/dexpace/go-sdk/jsonl"
)

func ExampleDecode() {
	type point struct {
		N int `json:"n"`
	}

	stream := strings.NewReader("{\"n\":1}\n{\"n\":2}\n")

	for p, err := range jsonl.Decode[point](stream) {
		if err != nil {
			fmt.Println("error:", err)
			return
		}
		fmt.Println(p.N)
	}
	// Output:
	// 1
	// 2
}
