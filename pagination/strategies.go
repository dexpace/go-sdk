// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package pagination

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// NewPageNumber returns a Pager that fetches sequentially numbered pages starting
// at startPage, advancing until fetch returns a page with no items. fetch is
// called with the page number and returns that page's items. Use [WithMaxPages]
// to bound iteration against APIs that may never return an empty page.
func NewPageNumber[T any](startPage int, fetch func(ctx context.Context, page int) ([]T, error), opts ...Option) *Pager[T] {
	tokenFetch := func(ctx context.Context, token string) (Page[T], error) {
		page := startPage
		if token != "" {
			n, err := strconv.Atoi(token)
			if err != nil {
				return Page[T]{}, fmt.Errorf("pagination: invalid page token %q: %w", token, err)
			}
			page = n
		}
		items, err := fetch(ctx, page)
		if err != nil {
			return Page[T]{}, err
		}
		next := ""
		if len(items) > 0 {
			next = strconv.Itoa(page + 1)
		}
		return Page[T]{Items: items, NextToken: next}, nil
	}
	return New(tokenFetch, opts...)
}

// NewLinkHeader returns a Pager that follows RFC 8288 Link headers. fetch is
// called with the next URL (empty for the first page) and returns the page's
// items and the HTTP response whose Link header carries the next URL. The Pager
// owns each returned response and closes its body after reading the Link header,
// so fetch must finish reading the body (to produce its items) before returning.
//
// The next URL is taken from the server-controlled Link header, so fetch should
// validate or trust the URL's host before dialing it to avoid SSRF.
func NewLinkHeader[T any](fetch func(ctx context.Context, url string) ([]T, *http.Response, error), opts ...Option) *Pager[T] {
	tokenFetch := func(ctx context.Context, token string) (Page[T], error) {
		items, resp, err := fetch(ctx, token)
		if err != nil {
			return Page[T]{}, err
		}
		next := NextLink(resp)
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return Page[T]{Items: items, NextToken: next}, nil
	}
	return New(tokenFetch, opts...)
}

// NextLink returns the URL of resp's RFC 8288 Link header entry whose rel
// includes "next", or "" when there is none (or resp is nil).
func NextLink(resp *http.Response) string {
	if resp == nil {
		return ""
	}
	for _, value := range resp.Header.Values("Link") {
		for _, entry := range splitLinkEntries(value) {
			url, rel := parseLinkEntry(entry)
			if url != "" && relHasNext(rel) {
				return url
			}
		}
	}
	return ""
}

// splitLinkEntries splits a Link header value on commas that are not inside the
// angle-bracketed URL of an entry.
func splitLinkEntries(value string) []string {
	var entries []string
	depth := 0
	start := 0
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				entries = append(entries, value[start:i])
				start = i + 1
			}
		}
	}
	entries = append(entries, value[start:])
	return entries
}

// parseLinkEntry extracts the <URL> and the rel parameter value from a single
// Link entry such as `<https://...>; rel="next"`.
func parseLinkEntry(entry string) (url, rel string) {
	entry = strings.TrimSpace(entry)
	open := strings.IndexByte(entry, '<')
	// IndexByte finds the first '>'. RFC 8288 requires URI-References to
	// percent-encode '>', so a literal '>' inside the URL is malformed input.
	closeIdx := strings.IndexByte(entry, '>')
	if open != 0 || closeIdx < 0 {
		return "", ""
	}
	url = entry[open+1 : closeIdx]
	for _, param := range strings.Split(entry[closeIdx+1:], ";") {
		param = strings.TrimSpace(param)
		name, val, ok := strings.Cut(param, "=")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(name), "rel") {
			rel = strings.Trim(strings.TrimSpace(val), `"`)
		}
	}
	return url, rel
}

// relHasNext reports whether the space-separated rel value includes "next".
func relHasNext(rel string) bool {
	for _, token := range strings.Fields(rel) {
		if strings.EqualFold(token, "next") {
			return true
		}
	}
	return false
}
