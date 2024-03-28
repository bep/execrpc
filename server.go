package execrpc

import (
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"reflect"
	"sync"
	"time"

	"github.com/bep/execrpc/codecs"
	"golang.org/x/sync/errgroup"
)

const (
	// MessageStatusOK is the status code for a successful and complete message exchange.
	MessageStatusOK = iota

	// MessageStatusContinue is the status code for a message that should continue the conversation.
	MessageStatusContinue

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
func NewServer[Q, M, R any](opts ServerOptions[Q, M, R]) (*Server[Q, M, R], error) {
	if opts.Handle == nil {
		return nil, fmt.Errorf("opts: Handle function is required")
	}

	if opts.Codec == nil {
		codecName := os.Getenv(envClientCodec)
		var err error
		opts.Codec, err = codecs.ForName(codecName)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve codec from env variable %s with value %q (set by client); it can optionally be set in ServerOptions", envClientCodec, codecName)
		}
	}

	var rawServer *ServerRaw

	var qq Q
	createZero := makeZeroValueAndPointerToZeroValue(qq)

	callRaw := func(message Message, d Dispatcher) error {
		qv, v := createZero()
		q := qv.(Q)

		err := opts.Codec.Decode(message.Body, v)
		if err != nil {
			m := Message{
				Header: message.Header,
				Body:   []byte(fmt.Sprintf("failed to decode request: %s. Check that client and server uses the same codec.", err)),
			}
			m.Header.Status = MessageStatusErrDecodeFailed
			d.SendMessage(m)
			return nil
		}

		call := &Call[Q, M, R]{
			Request:      q,
			messagesRaw:  make(chan Message, 10),
			messages:     make(chan M, 10),
			receiptFinal: make(chan R, 1),
		}

		go func() {
			opts.Handle(call)
			if !call.closed {
				// The server returned without calling Close.
				// This is OK, it just means that the server gave away
				// the opportunity to set the receipt values.
				call.close()
			}
		}()

		// Handle standalone messages in its own goroutine.
		go func() {
			for message := range call.messagesRaw {
				rawServer.dispatcher.SendMessage(message)
			}
		}()

		var size uint32
		var hasher hash.Hash
		if opts.GetHasher != nil {
			hasher = opts.GetHasher()
		}

		var shouldHash bool
		if hasher != nil {
			// Avoid hashing if the receipt does not implement Sum64Provider.
			var r *R
			_, shouldHash = any(r).(TagProvider)
		}

		defer func() {
			b, err := opts.Codec.Encode(call.receiptFromServer)
			h := message.Header
			h.Status = MessageStatusOK
			d.SendMessage(createMessage(b, err, h, MessageStatusErrEncodeFailed))
		}()

		var checksum string

		for m := range call.messages {
			b, err := opts.Codec.Encode(m)
			h := message.Header
			h.Status = MessageStatusContinue
			m := createMessage(b, err, h, MessageStatusErrEncodeFailed)
			d.SendMessage(m)
			if shouldHash {
				hasher.Write(m.Body)
			}
			size += uint32(len(m.Body))
		}
		if shouldHash {
			checksum = hex.EncodeToString(hasher.Sum(nil))
		}

		// TODO1 is pointer or not.
		setReceiptValuesIfNotSet(size, checksum, call.receiptFromServer)

		return nil
	}

	var err error
	rawServer, err = NewServerRaw(
		ServerRawOptions{
			Call: callRaw,
		},
	)
	if err != nil {
		return nil, err
	}

	return &Server[Q, M, R]{
		ServerRaw: rawServer,
	}, nil
}

func makeZeroValueAndPointerToZeroValue[T any](v T) func() (any, any) {
	if typ := reflect.TypeOf(v); typ.Kind() == reflect.Ptr {
		elem := typ.Elem()
		return func() (any, any) {
			vv := reflect.New(elem).Interface()
			return vv, vv
		}
	} else {
		return func() (any, any) { return v, &v }
	}
}

func setReceiptValuesIfNotSet(size uint32, checksum string, r any) {
	if m, ok := any(r).(LastModifiedProvider); ok && m.GetELastModified() == 0 {
		m.SetELastModified(time.Now().Unix())
	}
	if size != 0 {
		if m, ok := any(r).(SizeProvider); ok && m.GetESize() == 0 {
			m.SetESize(size)
		}
	}
	if checksum != "" {
		if m, ok := any(r).(TagProvider); ok && m.GetETag() == "" {
			m.SetETag(checksum)
		}
	}
}

func createMessage(b []byte, err error, h Header, failureStatus uint16) Message {
	var m Message
	if err != nil {
		m = Message{
			Header: h,
			Body:   []byte(fmt.Sprintf("failed create message: %s. Check that client and server uses the same codec.", err)),
		}
		m.Header.Status = failureStatus
	} else {
		m = Message{
			Header: h,
			Body:   b,
		}
	}
	return m
}

// ServerOptions is the options for a server.
type ServerOptions[Q, M, R any] struct {
	// Handle is the function that will be called when a request is received.
	Handle func(*Call[Q, M, R])

	// Codec is the codec that will be used to encode and decode requests, messages and receipts.
	// The client will tell the server what codec is in use, so in most cases you should just leave this unset.
	Codec codecs.Codec

	// GetHasher returns the hash instance to be used for the response body
	// If it's not set or it returns nil, no hash will be calculated.
	GetHasher func() hash.Hash
}

// Server is a stringly typed server for requests of type Q and responses of tye R.
type Server[Q, M, R any] struct {
	*ServerRaw
}

// ServerRaw is a RPC server handling raw messages with a header and []byte body.
// See Server for a generic, typed version.
type ServerRaw struct {
	call       func(Message, Dispatcher) error
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
	if !s.started {
		panic("server not started")
	}
	err := s.g.Wait()
	if s.onStop != nil {
		s.onStop()
	}
	return err
}

// inputOutput reads messages from the stdin and calls the server's call function.
// The response is written to stdout.
func (s *ServerRaw) inputOutput() error {
	// We currently treat all errors in here as stop signals.
	// This means that the server will stop taking requests and
	// needs to be restarted.
	// Server implementations should communicate client error situations
	// via the messages.
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

		err = s.call(
			Message{
				Header: header,
				Body:   body,
			},
			s.dispatcher,
		)
		if err != nil {
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
	// Message passed to the Dispatcher as part of the request/response must
	// use the same ID as the request.
	// ID 0 is reserved for standalone messages (e.g. log messages).
	Call func(Message, Dispatcher) error
}

type messageDispatcher struct {
	s *ServerRaw
}

// Call is the request/response exchange between the client and server.
// Note that the stream parameter S is optional, set it to any if not used.
type Call[Q, M, R any] struct {
	Request           Q
	messages          chan M
	messagesRaw       chan Message
	receiptFromServer R
	receiptFinal      chan R
	closed            bool
}

// TODO1 delete me.
type Receipt struct {
	Identity
}

// SendRaw sends one or more messages back to the client
// that is not part of the request/response exchange.
// These messages must have ID 0.
func (c *Call[Q, M, R]) SendRaw(ms ...Message) {
	for _, m := range ms {
		if m.Header.ID != 0 {
			panic("message ID must be 0 for standalone messages")
		}
		c.messagesRaw <- m
	}
}

// Send finalizes the request/response exchange.
// It returns a channel where the receipt can be read.
func (c *Call[Q, M, R]) Send(rr ...M) {
	for _, r := range rr {
		c.messages <- r
	}
}

func (c *Call[Q, M, R]) Close(r R) <-chan R {
	c.receiptFromServer = r
	c.close()
	return c.receiptFinal
}

func (c *Call[Q, M, R]) close() {
	close(c.messagesRaw)
	close(c.messages)
	c.closed = true
}

// Dispatcher is the interface for dispatching messages to the client.
type Dispatcher interface {
	// SendMessage sends one or more message back to the client.
	SendMessage(...Message)
}

func (s *messageDispatcher) SendMessage(ms ...Message) {
	for _, m := range ms {
		m.Header.Size = uint32(len(m.Body))
		if err := m.Write(s.s.out); err != nil {
			panic(err)
		}
	}
}
