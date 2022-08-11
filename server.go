package execrpc

import (
	"fmt"
	"io"

	"github.com/bep/execrpc/codecs"
	"golang.org/x/sync/errgroup"
)

func NewServerRaw(opts ServerRawOptions) (*ServerRaw, error) {
	if opts.Call == nil {
		return nil, fmt.Errorf("opts: Call function is required")
	}
	if opts.In == nil {
		return nil, fmt.Errorf("opts: In reader is required")
	}
	if opts.Out == nil {
		return nil, fmt.Errorf("opts: Out writer is required")
	}

	return &ServerRaw{
		call: opts.Call,
		in:   opts.In,
		out:  opts.Out,
	}, nil
}

func NewServer[Q, R any](opts ServerOptions[Q, R]) (*Server[Q, R], error) {
	if opts.Call == nil {
		return nil, fmt.Errorf("opts: Call function is required")
	}
	if opts.Codec == nil {
		return nil, fmt.Errorf("opts: Codec is required")
	}

	call := func(message Message) Message {
		var q Q
		err := opts.Codec.Decode(message.Body, &q)
		if err != nil {
			panic("TODO(bep)")
		}
		r := opts.Call(q)
		b, err := opts.Codec.Encode(r)
		if err != nil {
			panic("TODO(bep)")
		}
		return Message{
			Header: message.Header,
			Body:   b,
		}
	}

	rawServer, err := NewServerRaw(
		ServerRawOptions{
			Call: call,
			In:   opts.In,
			Out:  opts.Out,
		},
	)

	if err != nil {
		return nil, err
	}

	return &Server[Q, R]{
		ServerRaw: rawServer,
	}, nil
}

type ServerOptions[Q, R any] struct {
	Call  func(Q) R
	Codec codecs.Codec[Q, R]
	In    io.Reader
	Out   io.Writer
}

type Server[Q, R any] struct {
	*ServerRaw
}

// ServerRaw is a RPC server handling raw messages with a header and []byte body.
// See Server for a generic, typed version.
type ServerRaw struct {
	call func(Message) Message

	in  io.Reader
	out io.Writer

	g *errgroup.Group
}

func (s *ServerRaw) Start() error {
	s.g = &errgroup.Group{}

	s.g.Go(func() error {
		return s.readInOut()
	})

	return nil

}

func (s *ServerRaw) Wait() error {
	return s.g.Wait()
}

func (s *ServerRaw) readInOut() error {
	var err error
	for err == nil {
		var header Header
		if err = header.Read(s.in); err != nil {
			break
		}
		body := make([]byte, header.Size)
		_, err = io.ReadFull(s.in, body)
		if err != nil {
			break
		}

		response := s.call(
			Message{
				Header: header,
				Body:   body,
			},
		)
		response.Header.Size = uint32(len(response.Body))

		if err = response.Write(s.out); err != nil {
			break
		}
	}

	// TODO(bep) return real errors to caller.
	return err
}

type ServerRawOptions struct {
	Call func(message Message) Message
	In   io.Reader
	Out  io.Writer
}
