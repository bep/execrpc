// Copyright 2025 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package execrpc

import (
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
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

	// MessageStatusInitServer is the status code for a message used to initialize/configure the server.
	MessageStatusInitServer

	// MessageStatusErrDecodeFailed is the status code for a message that failed to decode.
	MessageStatusErrDecodeFailed
	// MessageStatusErrEncodeFailed is the status code for a message that failed to encode.
	MessageStatusErrEncodeFailed
	// MessageStatusErrInitServerFailed is the status code for a message that failed to initialize the server.
	MessageStatusErrInitServerFailed

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
func NewServer[C, Q, M, R any](opts ServerOptions[C, Q, M, R]) (*Server[C, Q, M, R], error) {
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

	var (
		rawServer   *ServerRaw
		messagesRaw = make(chan Message, 10)
	)

	callRaw := func(message Message, d Dispatcher) error {
		if message.Header.Status == MessageStatusInitServer {
			if opts.Init == nil {
				m := createErrorMessage(fmt.Errorf("opts: Init function is required"), message.Header, MessageStatusErrInitServerFailed)
				d.SendMessage(m)
				return nil
			}

			var (
				cfg          C
				protocolInfo = ProtocolInfo{Version: message.Header.Version}
			)
			err := opts.Codec.Decode(message.Body, &cfg)
			if err != nil {
				m := createErrorMessage(err, message.Header, MessageStatusErrDecodeFailed)
				d.SendMessage(m)
				return nil
			}

			if err := opts.Init(cfg, protocolInfo); err != nil {
				m := createErrorMessage(err, message.Header, MessageStatusErrInitServerFailed)
				d.SendMessage(m)
				return nil
			}

			// OK.
			var receipt Message
			receipt.Header = message.Header
			receipt.Header.Status = MessageStatusOK
			d.SendMessage(receipt)
			return nil
		}

		var q Q
		err := opts.Codec.Decode(message.Body, &q)
		if err != nil {
			m := createErrorMessage(err, message.Header, MessageStatusErrDecodeFailed)
			d.SendMessage(m)
			return nil
		}

		call := &Call[Q, M, R]{
			Request:           q,
			messagesRaw:       messagesRaw,
			messages:          make(chan M, 10),
			receiptToServer:   make(chan R, 1),
			receiptFromServer: make(chan R, 1),
		}

		go func() {
			opts.Handle(call)
			if !call.closed1 {
				// The server returned without fetching the Receipt.
				call.closeMessages()
			}
			if !call.closed2 {
				// The server did not call Close,
				// just send an empty receipt.
				var r R
				call.Close(false, r)
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

		var (
			checksum    string
			messageBuff []Message
		)

		defer func() {
			receipt := <-call.receiptFromServer

			// Send any buffered message before the receipt.
			if opts.DelayDelivery && !call.drop {
				for _, m := range messageBuff {
					d.SendMessage(m)
				}
			}

			b, err := opts.Codec.Encode(receipt)
			h := message.Header
			h.Status = MessageStatusOK
			d.SendMessage(createMessage(b, err, h, MessageStatusErrEncodeFailed))
		}()

		for m := range call.messages {
			b, err := opts.Codec.Encode(m)
			h := message.Header
			h.Status = MessageStatusContinue
			if h.ID == 0 {
				panic("message ID must not be 0 for request/response messages")
			}
			m := createMessage(b, err, h, MessageStatusErrEncodeFailed)
			if opts.DelayDelivery {
				messageBuff = append(messageBuff, m)
			} else {
				d.SendMessage(m)
			}
			if shouldHash {
				hasher.Write(m.Body)
			}
			size += uint32(len(m.Body))
		}
		if shouldHash {
			checksum = hex.EncodeToString(hasher.Sum(nil))
		}

		var receipt R
		setReceiptValuesIfNotSet(size, checksum, &receipt)

		call.receiptToServer <- receipt

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

	s := &Server[C, Q, M, R]{
		messagesRaw: messagesRaw,
		ServerRaw:   rawServer,
	}

	// Handle standalone messages in its own goroutine.
	go func() {
		for message := range s.messagesRaw {
			rawServer.dispatcher.SendMessage(message)
		}
	}()

	return s, nil
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
		return createErrorMessage(err, h, failureStatus)
	} else {
		m = Message{
			Header: h,
			Body:   b,
		}
	}
	return m
}

func createErrorMessage(err error, h Header, failureStatus uint16) Message {
	var additionalMsg string
	if failureStatus == MessageStatusErrDecodeFailed || failureStatus == MessageStatusErrEncodeFailed {
		additionalMsg = " Check that client and server uses the same codec."
	}
	m := Message{
		Header: h,
		Body:   fmt.Appendf(nil, "failed create message (error code %d): %s.%s", failureStatus, err, additionalMsg),
	}
	m.Header.Status = failureStatus
	return m
}

// ProtocolInfo is the protocol information passed to the server's Init function.
type ProtocolInfo struct {
	// The version passed down from the client.
	// This usually represents a major version,
	// so any increment should be considered a breaking change.
	Version uint16 `json:"version"`
}

// ServerOptions is the options for a server.
type ServerOptions[C, Q, M, R any] struct {
	// Init is the function that will be called when the server is started.
	// It can be used to initialize the server with the given configuration.
	// If an error is returned, the server will stop.
	Init func(C, ProtocolInfo) error

	// Handle is the function that will be called when a request is received.
	Handle func(*Call[Q, M, R])

	// Codec is the codec that will be used to encode and decode requests, messages and receipts.
	// The client will tell the server what codec is in use, so in most cases you should just leave this unset.
	Codec codecs.Codec

	// GetHasher returns the hash instance to be used for the response body
	// If it's not set or it returns nil, no hash will be calculated.
	GetHasher func() hash.Hash

	// Delay delivery of messages to the client until Close is called.
	// Close takes a drop parameter that will drop any buffered messages.
	// This can be useful if you want to check the server generated ETag,
	// maybe the client already has this data.
	DelayDelivery bool
}

// Server is a stringly typed server for requests of type Q and responses of tye R.
type Server[C, Q, M, R any] struct {
	messagesRaw chan Message
	*ServerRaw
}

func (s *Server[C, Q, M, R]) Start() error {
	err := s.ServerRaw.Start()

	// Close the standalone message channel.
	close(s.messagesRaw)

	if err == io.EOF {
		return nil
	}

	return err
}

// ServerRaw is a RPC server handling raw messages with a header and []byte body.
// See Server for a generic, typed version.
type ServerRaw struct {
	call       func(Message, Dispatcher) error
	dispatcher *messageDispatcher

	started bool
	onStop  func()

	in  io.Reader
	out io.Writer

	g *errgroup.Group
}

// Written by server to os.Stdout to signal it's ready for reading.
var serverStarted = []byte("_server_started")

// Start sets upt the server communication and starts the server loop.
func (s *ServerRaw) Start() error {
	if s.started {
		panic("server already started")
	}
	s.started = true

	// os.Stdout is where the client will listen for a specific byte stream,
	// and any writes to stdout outside of this protocol (e.g. fmt.Println("hello world!") will
	// freeze the server.
	//
	// To prevent that, we preserve the original stdout for the server and redirect user output to stderr.
	origStdout := os.Stdout
	done := make(chan bool)

	r, w, err := os.Pipe()
	if err != nil {
		return err
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

	err = s.g.Wait()
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
	mu sync.Mutex
	s  *ServerRaw
}

// Call is the request/response exchange between the client and server.
// Note that the stream parameter S is optional, set it to any if not used.
type Call[Q, M, R any] struct {
	Request           Q
	messagesRaw       chan Message
	messages          chan M
	receiptFromServer chan R
	receiptToServer   chan R

	closed1 bool // No more messages.
	closed2 bool // Receipt set.
	drop    bool // Drop buffered messages.
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

// Enqueue enqueues one or more messages to be sent back to the client.
func (c *Call[Q, M, R]) Enqueue(rr ...M) {
	for _, r := range rr {
		c.messages <- r
	}
}

func (c *Call[Q, M, R]) Receipt() <-chan R {
	c.closeMessages()
	return c.receiptToServer
}

// Close closes the call and sends andy buffered messages and the receipt back to the client.
// If drop is true, the buffered messages are dropped.
// Note that drop is only relevant if the server is configured with DelayDelivery set to true.
func (c *Call[Q, M, R]) Close(drop bool, r R) {
	c.drop = drop
	c.closed2 = true
	c.receiptFromServer <- r
}

func (c *Call[Q, M, R]) closeMessages() {
	c.closed1 = true
	close(c.messages)
}

// Dispatcher is the interface for dispatching messages to the client.
type Dispatcher interface {
	// SendMessage sends one or more message back to the client.
	SendMessage(...Message)
}

func (s *messageDispatcher) SendMessage(ms ...Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range ms {
		m.Header.Size = uint32(len(m.Body))
		if err := m.Write(s.s.out); err != nil {
			panic(err)
		}
	}
}
