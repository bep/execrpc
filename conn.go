package execrpc

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// ErrTimeoutWaitingForServer is returned on timeouts starting the server.
var ErrTimeoutWaitingForServer = errors.New("timed out waiting for server to start")

var brokenPipeRe = regexp.MustCompile("Broken pipe|pipe is being closed")

func newConn(cmd *exec.Cmd, timeout time.Duration) (_ conn, err error) {
	in, err := cmd.StdinPipe()
	if err != nil {
		return conn{}, err
	}
	defer func() {
		if err != nil {
			in.Close()
		}
	}()

	out, err := cmd.StdoutPipe()
	stdErr := &tailBuffer{limit: 1024}
	c := conn{
		ReadCloser:  out,
		WriteCloser: in,
		stdErr:      stdErr,
		cmd:         cmd,
		timeout:     timeout,
	}
	cmd.Stderr = io.MultiWriter(c.stdErr, os.Stderr)

	return c, err
}

type conn struct {
	io.ReadCloser
	io.WriteCloser
	stdErr *tailBuffer
	cmd    *exec.Cmd

	timeout time.Duration
}

// Close closes conn's WriteCloser, ReadClosers, and waits for the command to finish.
func (c conn) Close() error {
	writeErr := c.WriteCloser.Close()
	readErr := c.ReadCloser.Close()
	cmdErr := c.waitWithTimeout()

	if writeErr != nil {
		return writeErr
	}

	if readErr != nil {
		return readErr
	}

	return cmdErr
}

// Start starts conn's Cmd.
func (c conn) Start() error {
	err := c.cmd.Start()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		// THe server will announce when it's ready to read from stdin
		// by writing  a special string to stdout.
		for {
			select {
			case <-ctx.Done():
				return ErrTimeoutWaitingForServer
			default:
				done := make(chan bool)
				errc := make(chan error)
				go func() {
					var read []byte
					br := bufio.NewReader(c)
					for {
						select {
						case <-ctx.Done():
							return
						default:
							b, err := br.ReadByte()
							if err != nil {
								errc <- err
								break
							}
							read = append(read, b)
							if bytes.Contains(read, serverStarted) {
								remainder := bytes.Replace(read, serverStarted, nil, 1)
								if len(remainder) > 0 {
									os.Stdout.Write(remainder)
								}
								done <- true
								return
							}
						}
					}
				}()

				select {
				case <-ctx.Done():
					return ErrTimeoutWaitingForServer
				case err := <-errc:
					return err
				case <-done:
					return nil
				}
			}
		}
	})

	return g.Wait()
}

// the server ends itself on EOF, this is just to give it some
// time to do so.
func (c conn) waitWithTimeout() error {
	result := make(chan error, 1)
	go func() { result <- c.cmd.Wait() }()
	select {
	case err := <-result:
		if _, ok := err.(*exec.ExitError); ok {
			if brokenPipeRe.MatchString(c.stdErr.String()) {
				return nil
			}
		}
		return err
	case <-time.After(time.Second):
		return errors.New("timed out waiting for server to finish")
	}
}

type tailBuffer struct {
	mu sync.Mutex

	limit int
	buff  bytes.Buffer
}

func (b *tailBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(p)+b.buff.Len() > b.limit {
		b.buff.Reset()
	}
	n, err = b.buff.Write(p)
	return
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buff.String()
}
