package execrpc

import (
	"fmt"
	"io"
	"os"

	"github.com/bep/execrpc/codecs"
	"golang.org/x/sync/errgroup"
)

func NewServerRaw(opts ServerRawOptions) (*ServerRaw, error) {
	if opts.Call == nil {
		return nil, fmt.Errorf("opts: Call function is required")
	}

	origStdout := os.Stdout
	done := make(chan bool)

	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}

	// Prevent server implementations from freezing the server when writing to stdout (this will happen).
	os.Stdout = w

	go func() {
		// Copy all output from the pipe to stderr
		_, _ = io.Copy(os.Stderr, r)
		done <- true

	}()

	return &ServerRaw{
		call: opts.Call,
		onStop: func() {
			// Close one side of the pipe.
			_ = w.Close()
			<-done
		},
		in:  os.Stdin,
		out: origStdout,
	}, nil
}

func NewServer[Q, R any](opts ServerOptions[Q, R]) (*Server[Q, R], error) {
	if opts.Call == nil {
		return nil, fmt.Errorf("opts: Call function is required")
	}
	if opts.Codec == nil {
		return nil, fmt.Errorf("opts: Codec is required")
	}

	call := func(message Message) (Message, error) {
		var q Q
		err := opts.Codec.Decode(message.Body, &q)
		if err != nil {
			return Message{}, fmt.Errorf("failed to decode request: %s", err)
		}
		r := opts.Call(q)
		b, err := opts.Codec.Encode(r)
		if err != nil {
			return Message{}, fmt.Errorf("failed to encode response: %s", err)
		}
		return Message{
			Header: message.Header,
			Body:   b,
		}, nil
	}

	rawServer, err := NewServerRaw(
		ServerRawOptions{
			Call: call,
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
	Codec codecs.Codec[R, Q]
}

type Server[Q, R any] struct {
	*ServerRaw
}

// ServerRaw is a RPC server handling raw messages with a header and []byte body.
// See Server for a generic, typed version.
type ServerRaw struct {
	call func(Message) (Message, error)

	onStop func()

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
	err := s.g.Wait()
	if s.onStop != nil {
		s.onStop()
	}

	return err
}

func (s *ServerRaw) readInOut() error {
	// We currently treat all errors in here as fatal.
	// This means that the server will stop taking requests and
	// needs to be restarted.
	// Server implementations should communicate client error situations
	// via the response message.
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

		var response Message
		response, err = s.call(
			Message{
				Header: header,
				Body:   body,
			},
		)

		if err != nil {
			break
		}

		response.Header.Size = uint32(len(response.Body))

		if err = response.Write(s.out); err != nil {
			break
		}
	}

	return err
}

type ServerRawOptions struct {
	// Call is the message exhcange between the client and server.
	// Note that any error returned by this function will be treated as a fatal error and the server is stopped.
	// Validation errors etc. should be returned in the response message.
	Call func(message Message) (Message, error)
}
