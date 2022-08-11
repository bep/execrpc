package execrpc

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/bep/execrpc/codecs"
	"golang.org/x/sync/errgroup"
)

const (
	// MessageStatusOK is the status code for a successful message.
	MessageStatusOK = iota
	// MessageStatusErrDecodeFailed is the status code for a message that failed to decode.
	MessageStatusErrDecodeFailed
	// MessageStatusErrEncodeFailed is the status code for a message that failed to encode.
	MessageStatusErrEncodeFailed

	// MessageStatusSystemReservedMax is the maximum value for a system reserved status code.
	MessageStatusSystemReservedMax = 100
)

// NewServerRaw creates a new Server. using the given options.
func NewServerRaw(opts ServerRawOptions) (*ServerRaw, error) {
	if opts.Call == nil {
		return nil, fmt.Errorf("opts: Call function is required")
	}

	return &ServerRaw{
		call: opts.Call,
	}, nil
}

// NewServer creates a new Server. using the given options.
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

// ServerOptions is the options for a server.
type ServerOptions[Q, R any] struct {
	Call  func(Q) R
	Codec codecs.Codec[R, Q]
}

// Server is a stringly typed server for requests of type Q and responses of tye R.
type Server[Q, R any] struct {
	*ServerRaw
}

// ServerRaw is a RPC server handling raw messages with a header and []byte body.
// See Server for a generic, typed version.
type ServerRaw struct {
	call func(Message) (Message, error)

	startInit sync.Once
	onStop    func()

	in  io.Reader
	out io.Writer

	g *errgroup.Group
}

// Written by server to os.Stdout to signal it's ready for reading.
var serverStarted = []byte("_server_started")

// Start sets upt the server communication and starts the server loop.
// It's safe to call Start multiple times, but only the first call will start the server.
func (s *ServerRaw) Start() error {
	var initErr error
	s.startInit.Do(func() {
		// os.Stdout is where the client will listen for a specific byte stream,
		// and any writes to stdout outside of this protocol (e.g. fmt.Println("hello world!") will
		// freeze the server.
		//
		// To prevent that, we preserve the original stdout for the server and redirect user output to stderr.
		origStdout := os.Stdout
		done := make(chan bool)

		r, w, err := os.Pipe()
		if err != nil {
			initErr = err
			return
		}

		os.Stdout = w

		go func() {
			// Copy all output from the pipe to stderr.
			_, _ = io.Copy(os.Stderr, r)
			// Done when the pipe is closed.
			done <- true
		}()

		s.in = os.Stdin
		s.out = origStdout
		s.onStop = func() {
			// Close one side of the pipe.
			_ = w.Close()
			<-done
		}

		s.g = &errgroup.Group{}

		// Signal to client that the server is ready.
		fmt.Fprint(s.out, string(serverStarted)+"\n")

		s.g.Go(func() error {
			return s.inputOutput()
		})
	})

	return initErr
}

// Wait waits for the server to stop.
// This happens when it gets disconnected from the client.
func (s *ServerRaw) Wait() error {
	err := s.g.Wait()
	if s.onStop != nil {
		s.onStop()
	}
	return err
}

// Send sends a message to the client.
// This is normally used for log messages and similar,
// and these messages should have a zero (0) ID.
// Replies to requests are normally handled in Call.
func (s *ServerRaw) Send(message Message) {
	message.Header.Size = uint32(len(message.Body))
	if err := message.Write(s.out); err != nil {
		panic("failed to send message: " + err.Error()) // TODO(bep) set up a channel for these messages.
	}
}

// inputOutput reads messages from the stdin and calls the server's call function.
// The response is written to stdout.
func (s *ServerRaw) inputOutput() error {
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
