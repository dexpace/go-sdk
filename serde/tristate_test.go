// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package serde_test

import (
	"encoding/json"
	"testing"

	"github.com/dexpace/go-sdk/serde"
)

func TestTristateStates(t *testing.T) {
	t.Parallel()

	a := serde.Absent[string]()
	if !a.IsAbsent() || a.IsNull() || a.IsPresent() {
		t.Fatal("Absent state wrong")
	}
	if !a.IsZero() {
		t.Fatal("Absent should report IsZero")
	}

	n := serde.Null[string]()
	if !n.IsNull() || n.IsAbsent() || n.IsPresent() || n.IsZero() {
		t.Fatal("Null state wrong")
	}

	p := serde.Present("hi")
	if !p.IsPresent() || p.IsAbsent() || p.IsNull() || p.IsZero() {
		t.Fatal("Present state wrong")
	}
	if v, ok := p.Get(); !ok || v != "hi" {
		t.Fatalf("Get = (%q,%v), want (hi,true)", v, ok)
	}
	if _, ok := a.Get(); ok {
		t.Fatal("Absent Get should be (zero,false)")
	}
	if _, ok := n.Get(); ok {
		t.Fatal("Null Get should be (zero,false)")
	}
}

func TestTristateMarshalInStruct(t *testing.T) {
	t.Parallel()

	type patch struct {
		Name  serde.Tristate[string] `json:"name,omitzero"`
		Email serde.Tristate[string] `json:"email,omitzero"`
		Age   serde.Tristate[int]    `json:"age,omitzero"`
	}

	p := patch{
		Name:  serde.Present("alice"),
		Email: serde.Null[string](),
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != `{"name":"alice","email":null}` {
		t.Fatalf("Marshal = %s, want {\"name\":\"alice\",\"email\":null}", data)
	}
}

func TestTristateUnmarshal(t *testing.T) {
	t.Parallel()

	type patch struct {
		Name  serde.Tristate[string] `json:"name,omitzero"`
		Email serde.Tristate[string] `json:"email,omitzero"`
		Age   serde.Tristate[int]    `json:"age,omitzero"`
	}

	var p patch
	if err := json.Unmarshal([]byte(`{"name":"bob","email":null}`), &p); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if v, ok := p.Name.Get(); !ok || v != "bob" {
		t.Fatalf("Name = (%q,%v), want (bob,true)", v, ok)
	}
	if !p.Email.IsNull() {
		t.Fatal("Email should be Null")
	}
	if !p.Age.IsAbsent() {
		t.Fatal("Age should be Absent (omitted from input)")
	}
}
