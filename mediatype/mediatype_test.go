// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package mediatype_test

import (
	"testing"

	"github.com/dexpace/go-sdk/mediatype"
)

func TestParse(t *testing.T) {
	t.Parallel()

	mt, err := mediatype.Parse("application/JSON; charset=UTF-8")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if mt.Type != "application" || mt.Subtype != "json" {
		t.Fatalf("essence = %q, want application/json", mt.Essence())
	}
	if got := mt.Charset(); got != "UTF-8" {
		t.Fatalf("charset = %q, want UTF-8", got)
	}
	if !mt.Matches(mediatype.ApplicationJSON) {
		t.Fatal("parsed type should match ApplicationJSON")
	}
}

func TestParseInvalid(t *testing.T) {
	t.Parallel()

	if _, err := mediatype.Parse("not a media type; ;"); err == nil {
		t.Fatal("expected error for malformed media type")
	}
}

func TestStringRoundTrip(t *testing.T) {
	t.Parallel()

	mt := mediatype.TextPlain.WithParameter("charset", "utf-8")

	// Re-parse String() instead of asserting exact spacing, so the test does not
	// depend on mime.FormatMediaType's separator formatting.
	reparsed, err := mediatype.Parse(mt.String())
	if err != nil {
		t.Fatalf("re-parse %q: %v", mt.String(), err)
	}
	if !reparsed.Matches(mediatype.TextPlain) {
		t.Fatalf("essence = %q, want text/plain", reparsed.Essence())
	}
	if got := reparsed.Charset(); got != "utf-8" {
		t.Fatalf("charset = %q, want utf-8", got)
	}
}

func TestZeroValueStringIsEmpty(t *testing.T) {
	t.Parallel()

	var mt mediatype.MediaType
	if got := mt.String(); got != "" {
		t.Fatalf("zero String() = %q, want empty", got)
	}
}

func TestWithParameterDoesNotMutateReceiver(t *testing.T) {
	t.Parallel()

	base := mediatype.ApplicationJSON
	derived := base.WithParameter("charset", "utf-8")
	if _, ok := base.Parameter("charset"); ok {
		t.Fatal("WithParameter mutated the receiver")
	}
	if c := derived.Charset(); c != "utf-8" {
		t.Fatalf("derived charset = %q, want utf-8", c)
	}
}
