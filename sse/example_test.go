// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package sse_test

import (
	"fmt"
	"strings"

	"github.com/dexpace/go-sdk/sse"
)

func ExampleParse() {
	stream := strings.NewReader("data: hello\n\ndata: world\n\n")

	for event, err := range sse.Parse(stream) {
		if err != nil {
			fmt.Println("error:", err)
			return
		}
		fmt.Println(event.Data)
	}
	// Output:
	// hello
	// world
}
