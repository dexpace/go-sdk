// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

// Package pagination provides a generic, transport-agnostic helper for consuming
// token-paginated API responses as Go range-over-func iterators.
package pagination

import (
	"context"
	"iter"
)

// Page is one page of results of type T together with the token needed to fetch
// the next page. A NextToken of "" marks the final page.
type Page[T any] struct {
	Items     []T
	NextToken string
}

// FetchFunc retrieves the page identified by token. The first call receives the
// empty token. A non-nil error ends iteration.
type FetchFunc[T any] func(ctx context.Context, token string) (Page[T], error)

// Option configures a [Pager].
type Option func(*pagerConfig)

type pagerConfig struct {
	maxPages int
}

// WithMaxPages caps how many pages a Pager yields. A value <= 0 means unlimited
// (the default).
func WithMaxPages(n int) Option {
	return func(c *pagerConfig) { c.maxPages = n }
}

// Pager lazily walks every page produced by a [FetchFunc].
type Pager[T any] struct {
	fetch    FetchFunc[T]
	maxPages int
}

// New returns a Pager driven by fetch. Options such as [WithMaxPages] tune it.
func New[T any](fetch FetchFunc[T], opts ...Option) *Pager[T] {
	var c pagerConfig
	for _, opt := range opts {
		opt(&c)
	}
	return &Pager[T]{fetch: fetch, maxPages: c.maxPages}
}

// Pages returns an iterator over successive pages. Iteration stops after the
// page whose NextToken is empty, when ctx is cancelled, when fetch returns an
// error (delivered as the second value of the final iteration), or when the
// WithMaxPages cap is reached. The iterator is single-pass.
func (p *Pager[T]) Pages(ctx context.Context) iter.Seq2[Page[T], error] {
	return func(yield func(Page[T], error) bool) {
		token := ""
		pages := 0
		for {
			if p.maxPages > 0 && pages >= p.maxPages {
				return
			}
			if err := ctx.Err(); err != nil {
				yield(Page[T]{}, err)
				return
			}
			page, err := p.fetch(ctx, token)
			if err != nil {
				yield(Page[T]{}, err)
				return
			}
			if !yield(page, nil) {
				return
			}
			pages++
			if page.NextToken == "" {
				return
			}
			token = page.NextToken
		}
	}
}

// Items returns an iterator that flattens every page into its individual items.
// On a fetch error the zero value of T is yielded with the error, then iteration
// stops.
func (p *Pager[T]) Items(ctx context.Context) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		for page, err := range p.Pages(ctx) {
			if err != nil {
				var zero T
				yield(zero, err)
				return
			}
			for _, item := range page.Items {
				if !yield(item, nil) {
					return
				}
			}
		}
	}
}
