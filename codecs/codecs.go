package codecs

import (
	"bytes"
	"encoding/json"

	"github.com/pelletier/go-toml/v2"
)

// Codec defines the interface for a two way conversion between Q  and R.
type Codec[Q, R any] interface {
	Decode([]byte, *Q) error
	Encode(R) ([]byte, error)
}

// TOMLCodec is a Codec that uses TOML as the underlying format.
type TOMLCodec[Q, R any] struct{}

func (c TOMLCodec[Q, R]) Decode(b []byte, q *Q) error {
	return toml.Unmarshal(b, q)
}

func (c TOMLCodec[Q, R]) Encode(r R) ([]byte, error) {
	var b bytes.Buffer
	enc := toml.NewEncoder(&b)
	if err := enc.Encode(r); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// JSONCodec is a Codec that uses JSON as the underlying format.
type JSONCodec[Q, R any] struct{}

func (c JSONCodec[Q, R]) Decode(b []byte, q *Q) error {
	return json.Unmarshal(b, q)
}

func (c JSONCodec[Q, R]) Encode(r R) ([]byte, error) {
	return json.Marshal(r)
}
