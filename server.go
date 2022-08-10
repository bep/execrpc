package execrpc

import (
	"io"

	"golang.org/x/sync/errgroup"
)

func NewServer(opts ServerOptions) *Server {
	if opts.Call == nil {
		panic("opts.Call is nil")
	}
	if opts.In == nil {
		panic("opts.In is nil")
	}
	if opts.Out == nil {
		panic("opts.Out is nil")
	}

	return &Server{
		call: opts.Call,
		in:   opts.In,
		out:  opts.Out,
	}
}

type Server struct {
	call func(message Message) Message
	//Decode func(message Message) (T, error)

	in  io.Reader
	out io.Writer

	g *errgroup.Group
}

func (s *Server) Start() error {
	s.g = &errgroup.Group{}

	s.g.Go(func() error {
		return s.readInOut()
	})

	return nil

}

func (s *Server) Wait() error {
	return s.g.Wait()
}

func (s *Server) readInOut() error {
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

type ServerOptions struct {
	Call func(message Message) Message
	In   io.Reader
	Out  io.Writer
}
