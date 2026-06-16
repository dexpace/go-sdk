// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package pagination_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dexpace/go-sdk/pagination"
)

// threePageFetch serves three fixed pages of integers.
func threePageFetch() pagination.FetchFunc[int] {
	return func(_ context.Context, token string) (pagination.Page[int], error) {
		switch token {
		case "":
			return pagination.Page[int]{Items: []int{1, 2}, NextToken: "p2"}, nil
		case "p2":
			return pagination.Page[int]{Items: []int{3, 4}, NextToken: "p3"}, nil
		case "p3":
			return pagination.Page[int]{Items: []int{5}, NextToken: ""}, nil
		default:
			return pagination.Page[int]{}, errors.New("unknown token: " + token)
		}
	}
}

func TestItemsFlattensAllPages(t *testing.T) {
	t.Parallel()

	pager := pagination.New(threePageFetch())

	var got []int
	for item, err := range pager.Items(context.Background()) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got = append(got, item)
	}

	want := []int{1, 2, 3, 4, 5}
	if len(got) != len(want) {
		t.Fatalf("items = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("items = %v, want %v", got, want)
		}
	}
}

func TestPagesStopsOnFetchError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("boom")
	pager := pagination.New(func(_ context.Context, token string) (pagination.Page[int], error) {
		if token == "" {
			return pagination.Page[int]{Items: []int{1}, NextToken: "next"}, nil
		}
		return pagination.Page[int]{}, wantErr
	})

	var pages, errs int
	for _, err := range pager.Pages(context.Background()) {
		if err != nil {
			errs++
			if !errors.Is(err, wantErr) {
				t.Fatalf("err = %v, want %v", err, wantErr)
			}
			continue
		}
		pages++
	}
	if pages != 1 || errs != 1 {
		t.Fatalf("pages = %d, errs = %d, want 1 and 1", pages, errs)
	}
}

func TestEarlyBreakStopsIteration(t *testing.T) {
	t.Parallel()

	calls := 0
	pager := pagination.New(func(_ context.Context, _ string) (pagination.Page[int], error) {
		calls++
		return pagination.Page[int]{Items: []int{calls}, NextToken: "more"}, nil
	})

	for item := range pager.Items(context.Background()) {
		_ = item
		break
	}
	if calls != 1 {
		t.Fatalf("fetch calls = %d, want 1 after early break", calls)
	}
}

func TestWithMaxPagesCapsIteration(t *testing.T) {
	t.Parallel()

	fetch := func(_ context.Context, _ string) (pagination.Page[int], error) {
		return pagination.Page[int]{Items: []int{1}, NextToken: "more"}, nil
	}
	pager := pagination.New(fetch, pagination.WithMaxPages(3))

	pages := 0
	for _, err := range pager.Pages(context.Background()) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		pages++
	}
	if pages != 3 {
		t.Fatalf("pages = %d, want 3 (capped)", pages)
	}
}

func TestWithMaxPagesAlsoCapsItems(t *testing.T) {
	t.Parallel()

	fetch := func(_ context.Context, _ string) (pagination.Page[int], error) {
		return pagination.Page[int]{Items: []int{1, 2}, NextToken: "more"}, nil
	}
	pager := pagination.New(fetch, pagination.WithMaxPages(2))

	count := 0
	for _, err := range pager.Items(context.Background()) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		count++
	}
	if count != 4 {
		t.Fatalf("items = %d, want 4 (2 pages capped)", count)
	}
}
