package execrpc

import (
	"fmt"
	"io"
	"os"

	"github.com/bep/execrpc/codecs"
	"golang.org/x/sync/errgroup"
)

const (
	MessageStatusOK = iota
	MessageStatusErrDecodeFailed
	MessageStatusErrEncodeFailed

	// MessageStatusSystemReservedMax is the maximum value for a system reserved status code.
	MessageStatusSystemReservedMax = 100
)

// NewServerRaw creates a new Server. using the given options.
// Note that we're passing message via stdin and stdout,
// so it's not possible to print anything to stdout inside a server (that will time out).
// Use stderr for loigging (e.gl via fmt.Printf). TODO(bep) fix this.
func NewServerRaw(opts ServerRawOptions) (*ServerRaw, error) {
	if opts.Call == nil {
		return nil, fmt.Errorf("opts: Call function is required")
	}

	return &ServerRaw{
		call: opts.Call,
		in:   os.Stdin,
		out:  os.Stdout,
	}, nil
}

// NewServer creates a new Server. using the given options.
// Note that we're passing message via stdin and stdout,
// so it's not possible to print anything to stdout inside a server (that will time out).
// Use stderr for loigging (e.gl via fmt.Printf).  TODO(bep) fix this.
// Also, you must make sure to use the same codec as the client.
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
			m := Message{
				Header: message.Header,
				Body:   []byte(fmt.Sprintf("failed to decode request: %s. Check that client and server uses the same codec.", err)),
			}
			m.Header.Status = MessageStatusErrDecodeFailed
			return m, nil
		}
		r := opts.Call(q)
		b, err := opts.Codec.Encode(r)
		if err != nil {

			m := Message{
				Header: message.Header,
				Body:   []byte(fmt.Sprintf("failed to encode response: %s. Check that client and server uses the same codec.", err)),
			}
			m.Header.Status = MessageStatusErrEncodeFailed
			return m, nil
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
