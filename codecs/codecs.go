package codecs

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"google.golang.org/protobuf/proto"
)

// Codec defines the interface for a two way conversion between Q and R.
type Codec interface {
	Encode(any) ([]byte, error)
	Decode([]byte, any) error
	Name() string
}

// ErrUnknownCodec is returned when no codec is found for the given name.
var ErrUnknownCodec = errors.New("unknown codec")

// ForName returns the codec for the given name or ErrUnknownCodec if no codec is found.
func ForName(name string) (Codec, error) {
	switch strings.ToLower(name) {
	case "toml":
		return TOMLCodec{}, nil
	case "json":
		return JSONCodec{}, nil
	case "protobuf":
		return ProtobufCodec{}, nil
	default:
		return nil, ErrUnknownCodec
	}
}

// TOMLCodec is a Codec that uses TOML as the underlying format.
type TOMLCodec struct{}

func (c TOMLCodec) Decode(b []byte, v any) error {
	return toml.Unmarshal(b, v)
}

func (c TOMLCodec) Encode(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := toml.NewEncoder(&b)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func (c TOMLCodec) Name() string {
	return "TOML"
}

// JSONCodec is a Codec that uses JSON as the underlying format.
type JSONCodec struct{}

func (c JSONCodec) Decode(b []byte, v any) error {
	return json.Unmarshal(b, v)
}

func (c JSONCodec) Encode(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (c JSONCodec) Name() string {
	return "JSON"
}

type ProtobufCodec struct{}

func (c ProtobufCodec) Decode(b []byte, v any) error {
	message := v.(proto.Message)
	return proto.Unmarshal(b, message)
}

func (c ProtobufCodec) Encode(v any) ([]byte, error) {
	message := v.(proto.Message)
	return proto.Marshal(message)
}

func (c ProtobufCodec) Name() string {
	return "Protobuf"
}
