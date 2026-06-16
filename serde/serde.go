// Copyright (c) 2026 dexpace and Omar Aljarrah.
// Licensed under the MIT License. See LICENSE in the repository root for details.

package serde

import "encoding/json"

// Marshaler encodes a value to bytes.
type Marshaler interface {
	Marshal(v any) ([]byte, error)
}

// Unmarshaler decodes bytes into v, which must be a non-nil pointer.
type Unmarshaler interface {
	Unmarshal(data []byte, v any) error
}

// Serde is a paired Marshaler and Unmarshaler.
type Serde interface {
	Marshaler
	Unmarshaler
}

// JSON is the default Serde, backed by encoding/json.
var JSON Serde = jsonSerde{}

type jsonSerde struct{}

func (jsonSerde) Marshal(v any) ([]byte, error)      { return json.Marshal(v) }
func (jsonSerde) Unmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }
