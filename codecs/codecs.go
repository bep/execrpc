package codecs

import (
	"bytes"
	"encoding/gob"
	"encoding/json"

	"github.com/pelletier/go-toml/v2"
)

// Codec defines the interface for a two way conversion between Q  and R.
type Codec[Q, R any] interface {
	Encode(Q) ([]byte, error)
	Decode([]byte, *R) error
}

// TOMLCodec is a Codec that uses TOML as the underlying format.
type TOMLCodec[Q, R any] struct{}

func (c TOMLCodec[Q, R]) Decode(b []byte, r *R) error {
	return toml.Unmarshal(b, r)
}

func (c TOMLCodec[Q, R]) Encode(q Q) ([]byte, error) {
	var b bytes.Buffer
	enc := toml.NewEncoder(&b)
	if err := enc.Encode(q); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// JSONCodec is a Codec that uses JSON as the underlying format.
type JSONCodec[Q, R any] struct{}

func (c JSONCodec[Q, R]) Decode(b []byte, r *R) error {
	return json.Unmarshal(b, r)
}

func (c JSONCodec[Q, R]) Encode(q Q) ([]byte, error) {
	return json.Marshal(q)
}

// GobCodec is a Codec that uses gob as the underlying format.
type GobCodec[Q, R any] struct{}

func (c GobCodec[Q, R]) Decode(b []byte, r *R) error {
	dec := gob.NewDecoder(bytes.NewReader(b))
	return dec.Decode(r)
}

func (c GobCodec[Q, R]) Encode(q Q) ([]byte, error) {
	var b bytes.Buffer
	enc := gob.NewEncoder(&b)
	err := enc.Encode(q)
	if err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
