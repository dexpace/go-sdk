// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package pagination_test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/dexpace/go-sdk/pagination"
)

func collect(t *testing.T, seq func(yield func(int, error) bool)) []int {
	t.Helper()
	var got []int
	for item, err := range seq {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got = append(got, item)
	}
	return got
}

func TestNewPageNumber(t *testing.T) {
	t.Parallel()

	fetch := func(_ context.Context, page int) ([]int, error) {
		switch page {
		case 1:
			return []int{1, 2}, nil
		case 2:
			return []int{3, 4}, nil
		case 3:
			return []int{5}, nil
		default:
			return nil, nil
		}
	}
	pager := pagination.NewPageNumber(1, fetch)
	got := collect(t, pager.Items(context.Background()))

	want := []int{1, 2, 3, 4, 5}
	if !slices.Equal(got, want) {
		t.Fatalf("items = %v, want %v", got, want)
	}
}

func TestNewPageNumberWithMaxPages(t *testing.T) {
	t.Parallel()

	// fetch never returns an empty page; WithMaxPages must stop iteration.
	fetch := func(_ context.Context, page int) ([]int, error) {
		return []int{page}, nil
	}
	pager := pagination.NewPageNumber(1, fetch, pagination.WithMaxPages(2))
	got := collect(t, pager.Items(context.Background()))

	if !slices.Equal(got, []int{1, 2}) {
		t.Fatalf("items = %v, want [1 2] (capped at 2 pages)", got)
	}
}

func TestNewLinkHeader(t *testing.T) {
	t.Parallel()

	respWithNext := func(next string) *http.Response {
		h := http.Header{}
		if next != "" {
			h.Set("Link", "<"+next+">; rel=\"next\"")
		}
		return &http.Response{Header: h}
	}

	fetch := func(_ context.Context, url string) ([]int, *http.Response, error) {
		switch url {
		case "":
			return []int{1, 2}, respWithNext("https://api.example.test/items?page=2"), nil
		case "https://api.example.test/items?page=2":
			return []int{3}, respWithNext(""), nil
		default:
			return nil, nil, context.Canceled
		}
	}
	pager := pagination.NewLinkHeader(fetch)
	got := collect(t, pager.Items(context.Background()))

	want := []int{1, 2, 3}
	if !slices.Equal(got, want) {
		t.Fatalf("items = %v, want %v", got, want)
	}
}

func TestNextLink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		link string
		want string
	}{
		{"single next", `<https://api/x?page=2>; rel="next"`, "https://api/x?page=2"},
		{"next and prev", `<https://api/p1>; rel="prev", <https://api/p3>; rel="next"`, "https://api/p3"},
		{"unquoted rel", `<https://api/n>; rel=next`, "https://api/n"},
		{"multiple rels", `<https://api/n>; rel="prev next"`, "https://api/n"},
		{"comma in url", `<https://api/x?a=1,2>; rel="next"`, "https://api/x?a=1,2"},
		{"bad entry then next", `JUNK, <https://api/n>; rel="next"`, "https://api/n"},
		{"no next", `<https://api/p>; rel="prev"`, ""},
		{"empty", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := http.Header{}
			if tc.link != "" {
				h.Set("Link", tc.link)
			}
			resp := &http.Response{Header: h}
			if got := pagination.NextLink(resp); got != tc.want {
				t.Fatalf("NextLink = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNextLinkNilResponse(t *testing.T) {
	t.Parallel()

	if got := pagination.NextLink(nil); got != "" {
		t.Fatalf("NextLink(nil) = %q, want empty", got)
	}
}
