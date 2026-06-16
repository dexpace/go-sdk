// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package jsonl_test

import (
	"strings"
	"testing"

	"github.com/dexpace/go-sdk/jsonl"
)

type rec struct {
	N int `json:"n"`
}

func collectRecs(t *testing.T, input string) ([]rec, error) {
	t.Helper()
	var got []rec
	var gotErr error
	for v, err := range jsonl.Decode[rec](strings.NewReader(input)) {
		if err != nil {
			gotErr = err
			break
		}
		got = append(got, v)
	}
	return got, gotErr
}

func TestDecodeNDJSON(t *testing.T) {
	t.Parallel()

	got, err := collectRecs(t, "{\"n\":1}\n{\"n\":2}\n{\"n\":3}\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 || got[0].N != 1 || got[1].N != 2 || got[2].N != 3 {
		t.Fatalf("got %v, want n=1,2,3", got)
	}
}

func TestDecodeSingleNoTrailingNewline(t *testing.T) {
	t.Parallel()

	got, err := collectRecs(t, "{\"n\":5}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].N != 5 {
		t.Fatalf("got %v, want one value n=5", got)
	}
}

func TestDecodeScalars(t *testing.T) {
	t.Parallel()

	var got []int
	for v, err := range jsonl.Decode[int](strings.NewReader("1 2 3")) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got = append(got, v)
	}
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("got %v, want [1 2 3]", got)
	}
}

func TestDecodeEmpty(t *testing.T) {
	t.Parallel()

	got, err := collectRecs(t, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want no values", got)
	}
}

func TestDecodeMalformedMidStream(t *testing.T) {
	t.Parallel()

	got, err := collectRecs(t, "{\"n\":1}\n{bad}\n")
	if err == nil {
		t.Fatal("expected a decode error for the malformed value")
	}
	if len(got) != 1 || got[0].N != 1 {
		t.Fatalf("got %v, want the first value before the error", got)
	}
}

func TestDecodeTruncated(t *testing.T) {
	t.Parallel()

	got, err := collectRecs(t, "{\"n\":1}\n{\"n\":")
	if err == nil {
		t.Fatal("expected an error for the truncated final value")
	}
	if len(got) != 1 || got[0].N != 1 {
		t.Fatalf("got %v, want the first value before the truncation", got)
	}
}

func TestDecodeEarlyBreak(t *testing.T) {
	t.Parallel()

	count := 0
	for range jsonl.Decode[rec](strings.NewReader("{\"n\":1}\n{\"n\":2}\n{\"n\":3}\n")) {
		count++
		break
	}
	if count != 1 {
		t.Fatalf("consumed %d values, want 1 after break", count)
	}
}
