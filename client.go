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
)

// ErrShutdown will be returned from Execute and Close if the client is or
// is about to be shut down.
var ErrShutdown = errors.New("connection is shut down")

func StartClient(opts ClientOptions) (*Client, error) {
	if opts.Timeout == 0 {
		opts.Timeout = time.Second * 10
	}

	cmd := exec.Command(opts.Cmd, opts.Args...)
	cmd.Stderr = os.Stderr

	conn, err := newConn(cmd)
	if err != nil {
		return nil, err
	}

	if err := conn.Start(); err != nil {
		return nil, err
	}

	client := &Client{
		version: opts.Version,
		timeout: opts.Timeout,
		conn:    conn,
		pending: make(map[uint32]*call),
	}

	go client.input()

	return client, nil
}

type Client struct {
	version uint8

	conn conn

	closing  bool
	shutdown bool

	timeout time.Duration

	// Protects the sending of messages to the server.
	sendMu sync.Mutex

	mu      sync.Mutex // Protects all below.
	seq     uint32
	pending map[uint32]*call
}

func (c *Client) Close() error {
	return c.conn.Close()
}

// Execute sends body to the server and returns the Message it receives.
// It's safe to call Execute from multiple goroutines.
func (c *Client) Execute(body []byte) (Message, error) {
	call, err := c.newCall(body)
	if err != nil {
		return Message{}, err
	}

	select {
	case call = <-call.Done:
	case <-time.After(c.timeout):
		return Message{}, errors.New("timeout waiting for the server to respond")
	}

	return call.Response, call.Error
}

func (c *Client) newCall(body []byte) (*call, error) {
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

func (c *Client) input() {
	var err error

	for err == nil {
		var message Message
		err = message.Read(c.conn)
		if err != nil {
			break
		}
		id := message.Header.ID

		// Attach it to the correct pending call.
		c.mu.Lock()
		call := c.pending[id]
		delete(c.pending, id)
		c.mu.Unlock()
		if call == nil {
			err = fmt.Errorf("call with ID %d not found", id)
			break
		}
		call.Response = message
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

func (c *Client) send(call *call) error {
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

type ClientOptions struct {
	// Version number passed to the server.
	Version uint8

	// The server to start.
	Cmd string

	// The arguments to pass to the command.
	Args []string

	// The timeout for the client.
	Timeout time.Duration
}

type call struct {
	Request  Message
	Response Message
	Error    error
	Done     chan *call
}

func (call *call) done() {
	select {
	case call.Done <- call:
	default:
	}
}