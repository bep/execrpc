// Copyright 2025 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package execrpc_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/bep/execrpc"
	"github.com/bep/execrpc/codecs"
	"github.com/bep/execrpc/examples/model"
	qt "github.com/frankban/quicktest"
	"golang.org/x/sync/errgroup"
)

func TestRaw(t *testing.T) {
	c := qt.New(t)

	newClient := func(c *qt.C) *execrpc.ClientRaw {
		client, err := execrpc.StartClientRaw(
			execrpc.ClientRawOptions{
				Version: 1,
				Cmd:     "go",
				Dir:     "./examples/servers/raw",
				Args:    []string{"run", "."},
			})

		c.Assert(err, qt.IsNil)

		return client
	}

	c.Run("OK", func(c *qt.C) {
		client := newClient(c)
		defer client.Close()
		messages := make(chan execrpc.Message)
		var g errgroup.Group
		g.Go(func() error {
			return client.Execute(func(m *execrpc.Message) { m.Body = []byte("hello") }, messages)
		})
		var i int
		for msg := range messages {
			if i == 0 {
				c.Assert(string(msg.Body), qt.Equals, "echo: hello")
			}
			i++
		}
		c.Assert(i, qt.Equals, 1)
		c.Assert(g.Wait(), qt.IsNil)
	})
}

func TestStartFailed(t *testing.T) {
	c := qt.New(t)
	client, err := execrpc.StartClientRaw(
		execrpc.ClientRawOptions{
			Version: 1,
			Cmd:     "go",
			Dir:     "./examples/servers/doesnotexist",
			Args:    []string{"run", "."},
		})

	c.Assert(err, qt.IsNotNil)
	c.Assert(err.Error(), qt.Contains, "failed to start server: chdir ./examples/servers/doesnotexist")
	c.Assert(client.Close(), qt.IsNil)
}

const clientVersion = 3

func newTestClientForServer(t testing.TB, server string, codec codecs.Codec, cfg model.ExampleConfig, env ...string) *execrpc.Client[model.ExampleConfig, model.ExampleRequest, model.ExampleMessage, model.ExampleReceipt] {
	client, err := execrpc.StartClient(
		execrpc.ClientOptions[model.ExampleConfig, model.ExampleRequest, model.ExampleMessage, model.ExampleReceipt]{
			ClientRawOptions: execrpc.ClientRawOptions{
				Version: clientVersion,
				Cmd:     "go",
				Dir:     "./examples/servers/" + server,
				Args:    []string{"run", "."},
				Env:     env,
				Timeout: 30 * time.Second,
			},
			Config: cfg,
			Codec:  codec,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			if err != execrpc.ErrShutdown {
				t.Fatal(err)
			}
		}
	})

	return client
}

func newTestClient(t testing.TB, codec codecs.Codec, cfg model.ExampleConfig, env ...string) *execrpc.Client[model.ExampleConfig, model.ExampleRequest, model.ExampleMessage, model.ExampleReceipt] {
	return newTestClientForServer(t, "typed", codec, cfg, env...)
}

func TestTyped(t *testing.T) {
	c := qt.New(t)

	runBasicTestForClient := func(c *qt.C, client *execrpc.Client[model.ExampleConfig, model.ExampleRequest, model.ExampleMessage, model.ExampleReceipt]) execrpc.Result[model.ExampleMessage, model.ExampleReceipt] {
		result := client.Execute(model.ExampleRequest{Text: "world"})
		c.Assert(result.Err(), qt.IsNil)
		return result
	}

	assertMessages := func(c *qt.C, result execrpc.Result[model.ExampleMessage, model.ExampleReceipt], expected int) {
		var i int
		for m := range result.Messages() {
			expect := fmt.Sprintf("%d: Hello world!", i)
			c.Assert(string(m.Hello), qt.Equals, expect)
			i++
		}
		c.Assert(i, qt.Equals, expected)
		c.Assert(result.Err(), qt.IsNil)
	}

	c.Run("One message", func(c *qt.C) {
		client := newTestClient(c, codecs.JSONCodec{}, model.ExampleConfig{})
		result := runBasicTestForClient(c, client)
		assertMessages(c, result, 1)
		receipt := <-result.Receipt()
		c.Assert(receipt.GetESize(), qt.Equals, uint32(123))
		c.Assert(receipt.ETag, qt.Equals, "2d5537627636b58a")
		c.Assert(receipt.Text, qt.Equals, "echoed: world")
	})

	c.Run("100 messages", func(c *qt.C) {
		client := newTestClient(c, codecs.JSONCodec{}, model.ExampleConfig{NumMessages: 100})
		result := runBasicTestForClient(c, client)
		assertMessages(c, result, 100)
		receipt := <-result.Receipt()
		c.Assert(receipt.LastModified, qt.Not(qt.Equals), int64(0))
		c.Assert(receipt.ETag, qt.Equals, "15b8164b761923b7")
		c.Assert(receipt.Text, qt.Equals, "echoed: world")
	})

	c.Run("1234 messages", func(c *qt.C) {
		client := newTestClient(c, codecs.JSONCodec{}, model.ExampleConfig{NumMessages: 1234}, "EXECRPC_NUM_MESSAGES=1234")
		result := runBasicTestForClient(c, client)
		assertMessages(c, result, 1234)
		receipt := <-result.Receipt()
		c.Assert(result.Err(), qt.IsNil)
		c.Assert(receipt.LastModified, qt.Not(qt.Equals), int64(0))
		c.Assert(receipt.ETag, qt.Equals, "43940b97841cc686")
		c.Assert(receipt.Text, qt.Equals, "echoed: world")
	})

	c.Run("Delay delivery", func(c *qt.C) {
		client := newTestClient(c, codecs.JSONCodec{}, model.ExampleConfig{}, "EXECRPC_DELAY_DELIVERY=true")
		result := runBasicTestForClient(c, client)
		assertMessages(c, result, 1)
		receipt := <-result.Receipt()
		c.Assert(receipt.GetESize(), qt.Equals, uint32(123))
		c.Assert(receipt.Text, qt.Equals, "echoed: world")
	})

	c.Run("Delay delivery, drop messages", func(c *qt.C) {
		client := newTestClient(c, codecs.JSONCodec{}, model.ExampleConfig{DropMessages: true}, "EXECRPC_DELAY_DELIVERY=true")
		result := runBasicTestForClient(c, client)
		assertMessages(c, result, 0)
		receipt := <-result.Receipt()
		// This is a little confusing. We always get a receipt even if the messages are dropped,
		// and the server can create whatever protocol it wants.
		c.Assert(receipt.GetESize(), qt.Equals, uint32(123))
		c.Assert(receipt.ETag, qt.Equals, "2d5537627636b58a")
		c.Assert(receipt.Text, qt.Equals, "echoed: world")
	})

	c.Run("No Close", func(c *qt.C) {
		client := newTestClient(c, codecs.JSONCodec{}, model.ExampleConfig{NoClose: true})
		result := runBasicTestForClient(c, client)
		assertMessages(c, result, 1)
		receipt := <-result.Receipt()
		// Empty receipt.
		c.Assert(receipt.LastModified, qt.Equals, int64(0))
	})

	c.Run("Receipt", func(c *qt.C) {
		client := newTestClient(c, codecs.JSONCodec{}, model.ExampleConfig{})
		result := runBasicTestForClient(c, client)
		assertMessages(c, result, 1)
		receipt := <-result.Receipt()
		c.Assert(receipt.LastModified, qt.Not(qt.Equals), int64(0))
		c.Assert(receipt.ETag, qt.Equals, "2d5537627636b58a")

		// Set by the server.
		c.Assert(receipt.Text, qt.Equals, "echoed: world")
		c.Assert(receipt.Size, qt.Equals, uint32(123))
	})

	c.Run("No hasher", func(c *qt.C) {
		client := newTestClient(c, codecs.JSONCodec{}, model.ExampleConfig{}, "EXECRPC_NO_HASHER=true")
		result := runBasicTestForClient(c, client)
		assertMessages(c, result, 1)
		receipt := <-result.Receipt()
		c.Assert(receipt.ETag, qt.Equals, "")
	})

	c.Run("No reading Receipt", func(c *qt.C) {
		client := newTestClient(c, codecs.JSONCodec{}, model.ExampleConfig{NoReadingReceipt: true})
		result := runBasicTestForClient(c, client)
		assertMessages(c, result, 1)
		receipt := <-result.Receipt()
		// Empty receipt.
		c.Assert(receipt.LastModified, qt.Equals, int64(0))
	})

	c.Run("No reading Receipt, no Close", func(c *qt.C) {
		client := newTestClient(c, codecs.JSONCodec{}, model.ExampleConfig{NoClose: true, NoReadingReceipt: true})
		result := runBasicTestForClient(c, client)
		assertMessages(c, result, 1)
		receipt := <-result.Receipt()
		// Empty receipt.
		c.Assert(receipt.LastModified, qt.Equals, int64(0))
	})

	c.Run("Send log message from server", func(c *qt.C) {
		var logMessages []execrpc.Message

		client, err := execrpc.StartClient(
			execrpc.ClientOptions[model.ExampleConfig, model.ExampleRequest, model.ExampleMessage, model.ExampleReceipt]{
				ClientRawOptions: execrpc.ClientRawOptions{
					Version: clientVersion,
					Cmd:     "go",
					Dir:     "./examples/servers/typed",
					Args:    []string{"run", "."},
					Timeout: 30 * time.Second,
				},
				Config: model.ExampleConfig{SendLogMessage: true},
				Codec:  codecs.JSONCodec{},
			},
		)
		if err != nil {
			c.Fatal(err)
		}
		var wg errgroup.Group
		wg.Go(func() error {
			for msg := range client.MessagesRaw() {
				fmt.Println("got message", string(msg.Body))
				logMessages = append(logMessages, msg)
			}
			return nil
		})
		result := client.Execute(model.ExampleRequest{Text: "world"})
		c.Assert(result.Err(), qt.IsNil)
		assertMessages(c, result, 1)
		c.Assert(client.Close(), qt.IsNil)
		c.Assert(wg.Wait(), qt.IsNil)
		c.Assert(string(logMessages[0].Body), qt.Equals, "first log message")
		c.Assert(logMessages[0].Header.Status, qt.Equals, uint16(150))
		c.Assert(logMessages[0].Header.Version, qt.Equals, uint16(32))
	})

	c.Run("TOML", func(c *qt.C) {
		client := newTestClient(c, codecs.TOMLCodec{}, model.ExampleConfig{})
		result := runBasicTestForClient(c, client)
		assertMessages(c, result, 1)
	})

	c.Run("Error in receipt", func(c *qt.C) {
		client := newTestClient(c, codecs.JSONCodec{}, model.ExampleConfig{CallShouldFail: true})
		result := client.Execute(model.ExampleRequest{Text: "hello"})
		c.Assert(result.Err(), qt.IsNil)
		receipt := <-result.Receipt()
		c.Assert(receipt.Error, qt.Not(qt.IsNil))
		assertMessages(c, result, 0)
	})

	// The "stdout print tests" are just to make sure that the server behaves and does not hang.

	c.Run("Print to stdout outside server before", func(c *qt.C) {
		client := newTestClient(c, codecs.JSONCodec{}, model.ExampleConfig{}, "EXECRPC_PRINT_OUTSIDE_SERVER_BEFORE=true")
		runBasicTestForClient(c, client)
	})

	c.Run("Print to stdout inside server", func(c *qt.C) {
		client := newTestClient(c, codecs.JSONCodec{}, model.ExampleConfig{}, "EXECRPC_PRINT_INSIDE_SERVER=true")
		runBasicTestForClient(c, client)
	})

	c.Run("Print to stdout outside server before", func(c *qt.C) {
		client := newTestClient(c, codecs.JSONCodec{}, model.ExampleConfig{}, "EXECRPC_PRINT_OUTSIDE_SERVER_BEFORE=true")
		runBasicTestForClient(c, client)
	})

	c.Run("Print to stdout inside after", func(c *qt.C) {
		client := newTestClient(c, codecs.JSONCodec{}, model.ExampleConfig{}, "EXECRPC_PRINT_OUTSIDE_SERVER_AFTER=true")
		runBasicTestForClient(c, client)
	})
}

// Make sure that the README example compiles and runs.
func TestReadmeExample(t *testing.T) {
	c := qt.New(t)

	client := newTestClientForServer(c, "readmeexample", codecs.JSONCodec{}, model.ExampleConfig{})
	var wg errgroup.Group
	wg.Go(func() error {
		for msg := range client.MessagesRaw() {
			s := string(msg.Body)
			msg := fmt.Sprintf("got message: %s id: %d status: %d header size: %d actual size: %d", s, msg.Header.ID, msg.Header.Status, msg.Header.Size, len(s))
			if s != "log message" {
				return fmt.Errorf("unexpected message: %s", msg)
			}
		}
		return nil
	})
	result := client.Execute(model.ExampleRequest{Text: "world"})
	c.Assert(result.Err(), qt.IsNil)
	var hellos []string
	for m := range result.Messages() {
		hellos = append(hellos, m.Hello)
	}
	c.Assert(hellos, qt.DeepEquals, []string{"Hello 1!", "Hello 2!", "Hello 3!"})
	receipt := <-result.Receipt()
	c.Assert(receipt.LastModified, qt.Not(qt.Equals), int64(0))
	c.Assert(receipt.ETag, qt.Equals, "44af821b6bd180d0")
	c.Assert(receipt.Text, qt.Equals, "echoed: world")
	c.Assert(result.Err(), qt.IsNil)
	c.Assert(client.Close(), qt.IsNil)
	c.Assert(wg.Wait(), qt.IsNil)
}

func TestTypedConcurrent(t *testing.T) {
	client := newTestClient(t, codecs.JSONCodec{}, model.ExampleConfig{})
	var g errgroup.Group

	for i := range 100 {
		g.Go(func() error {
			for j := range 10 {
				text := fmt.Sprintf("%d-%d", i, j)
				result := client.Execute(model.ExampleRequest{Text: text})
				if err := result.Err(); err != nil {
					return err
				}
				var k int
				for response := range result.Messages() {
					expect := fmt.Sprintf("%d: Hello %s!", k, text)
					if string(response.Hello) != expect {
						return fmt.Errorf("unexpected result: %s", response.Hello)
					}
					k++
				}
				receipt := <-result.Receipt()
				if receipt.Text != "echoed: "+text {
					return fmt.Errorf("unexpected receipt: %s", receipt.Text)
				}
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkClient(b *testing.B) {
	const word = "World"

	runBenchmark := func(name string, codec codecs.Codec, cfg model.ExampleConfig, env ...string) {
		b.Run(name, func(b *testing.B) {
			client := newTestClient(b, codec, cfg, env...)
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					result := client.Execute(model.ExampleRequest{Text: word})
					if err := result.Err(); err != nil {
						b.Fatal(err)
					}
					for range result.Messages() {
					}
					<-result.Receipt()
					if err := result.Err(); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}

	runBenchmarksForCodec := func(codec codecs.Codec, cfg model.ExampleConfig) {
		runBenchmark("1 message "+codec.Name(), codec, cfg)
		cfg.NumMessages = 100
		runBenchmark("100 messages "+codec.Name(), codec, cfg)
	}
	runBenchmarksForCodec(codecs.JSONCodec{}, model.ExampleConfig{})
	runBenchmark("100 messages JSON, no hasher ", codecs.JSONCodec{}, model.ExampleConfig{NumMessages: 100}, "EXECRPC_NO_HASHER=true")
	runBenchmarksForCodec(codecs.TOMLCodec{}, model.ExampleConfig{})
}
