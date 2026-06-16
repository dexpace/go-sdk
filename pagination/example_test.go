// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package pagination_test

import (
	"context"
	"fmt"

	"github.com/dexpace/go-sdk/pagination"
)

func ExamplePager_Items() {
	// fetch returns two fixed in-memory pages. The first page points to the
	// second via its NextToken; the second has no NextToken, ending iteration.
	fetch := func(_ context.Context, token string) (pagination.Page[string], error) {
		switch token {
		case "":
			return pagination.Page[string]{Items: []string{"a", "b"}, NextToken: "page-2"}, nil
		default:
			return pagination.Page[string]{Items: []string{"c"}}, nil
		}
	}

	pager := pagination.New(fetch)

	var items []string
	for item, err := range pager.Items(context.Background()) {
		if err != nil {
			fmt.Println("error:", err)
			return
		}
		items = append(items, item)
	}

	fmt.Println(items)
	// Output:
	// [a b c]
}
