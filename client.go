package execrpc

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/bep/execrpc/codecs"
	"github.com/bep/helpers/envhelpers"
)

// ErrShutdown will be returned from Execute and Close if the client is or
// is about to be shut down.
var ErrShutdown = errors.New("connection is shut down")

const (
	// Signal to server about what codec to use.
	envClientCodec = "EXECRPC_CLIENT_CODEC"
)

// StartClient starts a client for the given options.
func StartClient[Q, M, R any](opts ClientOptions[Q, M, R]) (*Client[Q, M, R], error) {
	if opts.Codec == nil {
		return nil, errors.New("opts: Codec is required")
	}

	// Pass default settings to the server.
	envhelpers.SetEnvVars(&opts.Env, envClientCodec, opts.Codec.Name())

	rawClient, err := StartClientRaw(opts.ClientRawOptions)
	if err != nil {
		return nil, err
	}

	return &Client[Q, M, R]{
		rawClient: rawClient,
		opts:      opts,
	}, nil
}

// Client is a strongly typed RPC client.
type Client[Q, M, R any] struct {
	rawClient *ClientRaw
	opts      ClientOptions[Q, M, R]
}

// Result is the result of a request
// with zero or more messages and the receipt.
type Result[M, R any] struct {
	messages chan M
	receipt  chan R
	errc     chan error
}

// Messages returns the messages from the server.
func (r Result[M, R]) Messages() <-chan M {
	return r.messages
}

// Receipt returns the receipt from the server.
func (r Result[M, R]) Receipt() <-chan R {
	return r.receipt
}

// Err returns any error.
func (r Result[M, R]) Err() error {
	select {
	case err := <-r.errc:
		return err
	default:
		return nil
	}
}

func (r Result[M, R]) close() {
	close(r.messages)
	close(r.receipt)
}

// MessagesRaw returns the raw messages from the server.
// These are not connected to the request-response flow,
// typically used for log messages etc.
func (c *Client[Q, M, R]) MessagesRaw() <-chan Message {
	return c.rawClient.Messages
}

// Execute sends the request to the server and returns the result.
// You should check Err() both before and after reading from the messages and receipt channels.
func (c *Client[Q, M, R]) Execute(r Q) Result[M, R] {
	result := Result[M, R]{
		messages: make(chan M, 10),
		receipt:  make(chan R, 1),
		errc:     make(chan error, 1),
	}

	body, err := c.opts.Codec.Encode(r)
	if err != nil {
		result.errc <- fmt.Errorf("failed to encode request: %w", err)
		result.close()
		return result
	}

	go func() {
		defer func() {
			result.close()
		}()

		messagesRaw := make(chan Message, 10)
		go func() {
			err := c.rawClient.Execute(body, messagesRaw)
			if err != nil {
				result.errc <- fmt.Errorf("failed to execute: %w", err)
			}
		}()

		for message := range messagesRaw {
			if message.Header.Status > MessageStatusContinue && message.Header.Status <= MessageStatusSystemReservedMax {
				// All of these are currently error situations produced by the server.
				result.errc <- fmt.Errorf("%s (error code %d)", message.Body, message.Header.Status)
				return
			}

			if message.Header.Status == MessageStatusContinue {
				var resp M
				err = c.opts.Codec.Decode(message.Body, &resp)
				if err != nil {
					result.errc <- err
					return
				}
				result.messages <- resp
			} else {
				// Receipt.
				var rec R
				err = c.opts.Codec.Decode(message.Body, &rec)
				if err != nil {
					result.errc <- err
					return
				}
				result.receipt <- rec
				return
			}

		}
	}()

	return result
}

// Close closes the client.
func (c *Client[Q, M, R]) Close() error {
	return c.rawClient.Close()
}

// StartClientRaw starts a untyped client client for the given options.
func StartClientRaw(opts ClientRawOptions) (*ClientRaw, error) {
	if opts.Timeout == 0 {
		opts.Timeout = time.Second * 30
	}

	cmd := exec.Command(opts.Cmd, opts.Args...)
	cmd.Stderr = os.Stderr
	env := os.Environ()
	var keyVals []string
	for _, env := range opts.Env {
		key, val := envhelpers.SplitEnvVar(env)
		keyVals = append(keyVals, key, val)
	}
	if len(keyVals) > 0 {
		envhelpers.SetEnvVars(&env, keyVals...)
	}
	cmd.Env = env

	cmd.Dir = opts.Dir

	conn, err := newConn(cmd, opts.Timeout)
	if err != nil {
		return nil, err
	}

	if err := conn.Start(); err != nil {
		return nil, fmt.Errorf("failed to start server: %s: %s", err, conn.stdErr.String())
	}

	client := &ClientRaw{
		version:  opts.Version,
		timeout:  opts.Timeout,
		conn:     conn,
		pending:  make(map[uint32]*call),
		Messages: make(chan Message, 10),
	}

	go client.input()

	return client, nil
}

// ClientRaw is a raw RPC client.
// Raw means that the client doesn't do any type conversion, a byte slice is what you get.
type ClientRaw struct {
	version uint16

	conn conn

	closing  bool
	shutdown bool

	// Messages from the server that are not part of the request-response flow.
	Messages chan Message

	timeout time.Duration

	// Protects the sending of messages to the server.
	sendMu sync.Mutex

	mu      sync.Mutex // Protects all below.
	seq     uint32
	pending map[uint32]*call
}

// Close closes the server connection and waits for the server process to quit.
func (c *ClientRaw) Close() error {
	if c == nil {
		return nil
	}
	defer close(c.Messages)

	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closing {
		return ErrShutdown
	}
	c.closing = true

	err := c.conn.Close()

	return err
}

// Execute sends body to the server and sends any messages to the messages channel.
// It's safe to call Execute from multiple goroutines.
// The messages channel wil be closed when the call is done.
func (c *ClientRaw) Execute(body []byte, messages chan<- Message) error {
	defer close(messages)

	call, err := c.newCall(body, messages)
	if err != nil {
		return err
	}

	timer := time.NewTimer(c.timeout)
	defer timer.Stop()

	select {
	case call = <-call.Done:
	case <-timer.C:
		return ErrTimeoutWaitingForCall
	}

	if call.Error != nil {
		return c.addErrContext("execute", call.Error)
	}

	return nil
}

func (c *ClientRaw) addErrContext(op string, err error) error {
	return fmt.Errorf("%s: %s %s", op, err, c.conn.stdErr.String())
}

func (c *ClientRaw) newCall(body []byte, messages chan<- Message) (*call, error) {
	c.mu.Lock()
	c.seq++
	id := c.seq

	call := &call{
		Done: make(chan *call, 1),
		Request: Message{
			Header: Header{
				Version: c.version,
				ID:      id,
			},
			Body: body,
		},
		Messages: messages,
	}

	if c.shutdown || c.closing {
		c.mu.Unlock()
		call.Error = ErrShutdown
		call.done()
		return call, nil
	}

	c.pending[id] = call

	c.mu.Unlock()

	return call, c.send(call)
}

func (c *ClientRaw) input() {
	var err error

	for err == nil {
		var message Message
		err = message.Read(c.conn)
		if err != nil {
			break
		}

		id := message.Header.ID
		if id == 0 {
			// A message with ID 0 is a standalone message (e.g. log message)
			// and not part of the request-response flow.
			c.Messages <- message
			continue
		}

		// Attach it to the correct pending call.
		c.mu.Lock()
		call, found := c.pending[id]
		if !found {
			panic(fmt.Sprintf("call with ID %d not found", id))
		}
		if message.Header.Status == MessageStatusContinue {
			call.Messages <- message
			c.mu.Unlock()
			continue
		}

		delete(c.pending, id)
		c.mu.Unlock()
		if call == nil {
			err = fmt.Errorf("call with ID %d not found", id)
			break
		}
		call.Messages <- message
		call.done()
	}

	// Terminate pending calls.
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()

	c.shutdown = true
	isEOF := err == io.EOF || strings.Contains(err.Error(), "already closed")
	if isEOF {
		if c.closing {
			err = ErrShutdown
		} else {
			err = io.ErrUnexpectedEOF
		}
	}

	for _, call := range c.pending {
		call.Error = err
		call.done()
	}
}

func (c *ClientRaw) send(call *call) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	c.mu.Lock()
	if c.closing || c.shutdown {
		c.mu.Unlock()
		return ErrShutdown
	}
	c.mu.Unlock()
	return call.Request.Write(c.conn)
}

// ClientOptions are options for the client.
type ClientOptions[Q, M, R any] struct {
	ClientRawOptions
	Codec codecs.Codec
}

// ClientRawOptions are options for the raw part of the client.
type ClientRawOptions struct {
	// Version number passed to the server.
	Version uint16

	// The server to start.
	Cmd string

	// The arguments to pass to the command.
	Args []string

	// Environment variables to pass to the command.
	// These will be merged with the environment variables of the current process,
	// vallues in this slice have precedence.
	// A slice of strings of the form "key=value"
	Env []string

	// Dir specifies the working directory of the command.
	// If Dir is the empty string, the command runs in the
	// calling process's current directory.
	Dir string

	// The timeout for the client.
	Timeout time.Duration
}

var (
	_ TagProvider          = &Identity{}
	_ LastModifiedProvider = &Identity{}
	_ SizeProvider         = &Identity{}
)

// Identity holds the modified time (Unix seconds) and a 64-bit checksum.
type Identity struct {
	LastModified int64  `json:"lastModified"`
	ETag         string `json:"eTag"`
	Size         uint32 `json:"size"`
}

// GetETag returns the checksum.
func (i Identity) GetETag() string {
	return i.ETag
}

// SetETag sets the checksum.
func (i *Identity) SetETag(s string) {
	i.ETag = s
}

// GetELastModified returns the last modified time.
func (i Identity) GetELastModified() int64 {
	return i.LastModified
}

// SetELastModified sets the last modified time.
func (i *Identity) SetELastModified(t int64) {
	i.LastModified = t
}

// GetESize returns the size.
func (i Identity) GetESize() uint32 {
	return i.Size
}

// SetESize sets the size.
func (i *Identity) SetESize(s uint32) {
	i.Size = s
}

// TagProvider is the interface for a type that can provide a eTag.
type TagProvider interface {
	GetETag() string
	SetETag(string)
}

// LastModifiedProvider is the interface for a type that can provide a last modified time.
type LastModifiedProvider interface {
	GetELastModified() int64
	SetELastModified(int64)
}

// SizeProvider is the interface for a type that can provide a size.
type SizeProvider interface {
	GetESize() uint32
	SetESize(uint32)
}

type call struct {
	Request  Message
	Messages chan<- Message
	Error    error
	Done     chan *call
}

func (call *call) done() {
	select {
	case call.Done <- call:
	default:
	}
}
