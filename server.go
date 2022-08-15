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
	MessageStatusSystemReservedMax = 99
)

// NewServerRaw creates a new Server using the given options.
func NewServerRaw(opts ServerRawOptions) (*ServerRaw, error) {
	if opts.Call == nil {
		return nil, fmt.Errorf("opts: Call function is required")
	}
	s := &ServerRaw{
		call: opts.Call,
	}
	s.dispatcher = &messageDispatcher{
		s: s,
	}
	return s, nil
}

// NewServer creates a new Server. using the given options.
func NewServer[Q, R any](opts ServerOptions[Q, R]) (*Server[Q, R], error) {
	if opts.Call == nil {
		return nil, fmt.Errorf("opts: Call function is required")
	}

	if opts.Codec == nil {
		codecName := os.Getenv(envClientCodec)
		var err error
		opts.Codec, err = codecs.ForName[R, Q](codecName)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve codec from env variable %s with value %q (set by client); it can optionally be set in ServerOptions", envClientCodec, codecName)
		}
	}

	var rawServer *ServerRaw

	call := func(d Dispatcher, message Message) (Message, error) {
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
		r := opts.Call(rawServer.dispatcher, q)
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

	var err error
	rawServer, err = NewServerRaw(
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
	// Call is the function that will be called when a request is received.
	Call func(Dispatcher, Q) R

	// Codec is the codec that will be used to encode and decode requests and responses.
	// The client will tell the server what codec is in use, so in most cases you should just leave this unset.
	Codec codecs.Codec[R, Q]
}

// Server is a stringly typed server for requests of type Q and responses of tye R.
type Server[Q, R any] struct {
	*ServerRaw
}

// ServerRaw is a RPC server handling raw messages with a header and []byte body.
// See Server for a generic, typed version.
type ServerRaw struct {
	call       func(Dispatcher, Message) (Message, error)
	dispatcher *messageDispatcher

	startInit sync.Once
	started   bool
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
		defer func() {
			s.started = true
		}()

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

		s.g.Go(func() error {

			return nil
		})
	})

	return initErr
}

// Wait waits for the server to stop.
// This happens when it gets disconnected from the client.
func (s *ServerRaw) Wait() error {
	s.checkStarted()
	err := s.g.Wait()
	if s.onStop != nil {
		s.onStop()
	}
	return err
}

func (s *ServerRaw) checkStarted() {
	if !s.started {
		panic("server not started")
	}
}

// inputOutput reads messages from the stdin and calls the server's call function.
// The response is written to stdout.
func (s *ServerRaw) inputOutput() error {

	// We currently treat all errors in here as stop signals.
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
			s.dispatcher,
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

// ServerRawOptions is the options for a raw portion of the server.
type ServerRawOptions struct {
	// Call is the message exhcange between the client and server.
	// Note that any error returned by this function will be treated as a fatal error and the server is stopped.
	// Validation errors etc. should be returned in the response message.
	// The Dispatcher can be used to send messages to the client outside of the request/response loop, e.g. log messages.
	// Note that these messages must have ID 0.
	Call func(Dispatcher, Message) (Message, error)
}

type messageDispatcher struct {
	s *ServerRaw
}

// Dispatcher is the interface for dispatching standalone messages to the client, e.g. log messages.
type Dispatcher interface {
	// Send sends one or more message back to the client.
	// This is normally used for log messages and similar,
	// and these messages should have a zero (0) ID.
	Send(...Message) error
}

func (s *messageDispatcher) Send(messages ...Message) error {
	for _, message := range messages {
		if message.Header.ID != 0 {
			return fmt.Errorf("message ID must be 0")
		}
		message.Header.Size = uint32(len(message.Body))
		if err := message.Write(s.s.out); err != nil {
			return err
		}
	}
	return nil
}
