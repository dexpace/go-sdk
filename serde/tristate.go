// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package serde

import "encoding/json"

// tristate enumerates the three states of a [Tristate] value.
type tristate uint8

const (
	stateAbsent  tristate = iota // zero value: field not set
	stateNull                    // explicit JSON null
	statePresent                 // a concrete value
)

// Tristate distinguishes a field that is absent (omitted), explicitly null, or
// present with a value — the three states a JSON PATCH payload must express. The
// zero value is Absent.
//
// Tag a Tristate struct field with json:",omitzero" so an Absent value is omitted
// from the encoded output:
//
//	type PatchUser struct {
//		Name serde.Tristate[string] `json:"name,omitzero"`
//	}
type Tristate[T any] struct {
	state tristate
	value T
}

// Absent returns a Tristate with no value set (the zero value).
func Absent[T any]() Tristate[T] { return Tristate[T]{state: stateAbsent} }

// Null returns a Tristate representing an explicit null.
func Null[T any]() Tristate[T] { return Tristate[T]{state: stateNull} }

// Present returns a Tristate holding v.
func Present[T any](v T) Tristate[T] { return Tristate[T]{state: statePresent, value: v} }

// IsAbsent reports whether the value is absent (unset).
func (t Tristate[T]) IsAbsent() bool { return t.state == stateAbsent }

// IsNull reports whether the value is an explicit null.
func (t Tristate[T]) IsNull() bool { return t.state == stateNull }

// IsPresent reports whether a concrete value is present.
func (t Tristate[T]) IsPresent() bool { return t.state == statePresent }

// Get returns the value and whether it is present (a non-null, non-absent value).
func (t Tristate[T]) Get() (T, bool) {
	return t.value, t.state == statePresent
}

// IsZero reports whether the value is Absent. The json package consults IsZero
// for the ",omitzero" tag, so an Absent field is omitted from the output.
func (t Tristate[T]) IsZero() bool { return t.state == stateAbsent }

// MarshalJSON encodes a Present value as itself and Null as "null". An Absent
// value also encodes as "null"; tag the field json:",omitzero" to omit it
// instead.
func (t Tristate[T]) MarshalJSON() ([]byte, error) {
	if t.state == statePresent {
		return json.Marshal(t.value)
	}
	return []byte("null"), nil
}

// UnmarshalJSON decodes "null" as Null and any other value as Present. A field
// absent from the input is never passed here, so it remains Absent.
func (t *Tristate[T]) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		var zero T
		t.value = zero
		t.state = stateNull
		return nil
	}
	if err := json.Unmarshal(data, &t.value); err != nil {
		return err
	}
	t.state = statePresent
	return nil
}
