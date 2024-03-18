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

func TestExecRaw(t *testing.T) {
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
			return client.Execute([]byte("hello"), messages)
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

func TestExecStartFailed(t *testing.T) {
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

func newTestClient(t testing.TB, codec codecs.Codec, env ...string) *execrpc.Client[model.ExampleRequest, model.ExampleMessage, model.ExampleReceipt] {
	client, err := execrpc.StartClient(
		execrpc.ClientOptions[model.ExampleRequest, model.ExampleMessage, model.ExampleReceipt]{
			ClientRawOptions: execrpc.ClientRawOptions{
				Version: 1,
				Cmd:     "go",
				Dir:     "./examples/servers/typed",
				Args:    []string{"run", "."},
				Env:     env,
				Timeout: 30 * time.Second,
			},
			Codec: codec,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Fatal(err)
		}
	})

	return client
}

func TestExecTyped(t *testing.T) {
	c := qt.New(t)

	runBasicTestForClient := func(c *qt.C, client *execrpc.Client[model.ExampleRequest, model.ExampleMessage, model.ExampleReceipt]) execrpc.Result[model.ExampleMessage, model.ExampleReceipt] {
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
		client := newTestClient(c, codecs.JSONCodec{})
		result := runBasicTestForClient(c, client)
		assertMessages(c, result, 1)
		receipt := <-result.Receipt()
		c.Assert(receipt.GetESize(), qt.Equals, uint32(123))
	})

	c.Run("100 messages", func(c *qt.C) {
		client := newTestClient(c, codecs.JSONCodec{}, "EXECRPC_NUM_MESSAGES=100")
		result := runBasicTestForClient(c, client)
		assertMessages(c, result, 100)
		receipt := <-result.Receipt()
		c.Assert(receipt.LastModified, qt.Not(qt.Equals), int64(0))
		c.Assert(receipt.ETag, qt.Equals, "15b8164b761923b7")
	})

	c.Run("1234 messages", func(c *qt.C) {
		client := newTestClient(c, codecs.JSONCodec{}, "EXECRPC_NUM_MESSAGES=1234")
		result := runBasicTestForClient(c, client)
		assertMessages(c, result, 1234)
		receipt := <-result.Receipt()
		c.Assert(result.Err(), qt.IsNil)
		c.Assert(receipt.LastModified, qt.Not(qt.Equals), int64(0))
		c.Assert(receipt.ETag, qt.Equals, "43940b97841cc686")
	})

	c.Run("Delay delivery", func(c *qt.C) {
		client := newTestClient(c, codecs.JSONCodec{}, "EXECRPC_DELAY_DELIVERY=true")
		result := runBasicTestForClient(c, client)
		assertMessages(c, result, 1)
		receipt := <-result.Receipt()
		c.Assert(receipt.GetESize(), qt.Equals, uint32(123))
	})

	c.Run("Delay delivery, drop messages", func(c *qt.C) {
		client := newTestClient(c, codecs.JSONCodec{}, "EXECRPC_DELAY_DELIVERY=true", "EXECRPC_DROP_MESSAGES=true")
		result := runBasicTestForClient(c, client)
		assertMessages(c, result, 0)
		receipt := <-result.Receipt()
		// This is a little confusing. We always get a receipt even if the messages are dropped,
		// and the server can create whatever protocol it wants.
		c.Assert(receipt.GetESize(), qt.Equals, uint32(123))
	})

	c.Run("No Close", func(c *qt.C) {
		client := newTestClient(c, codecs.JSONCodec{}, "EXECRPC_NO_CLOSE=true")
		result := runBasicTestForClient(c, client)
		assertMessages(c, result, 1)
		receipt := <-result.Receipt()
		// Empty receipt.
		c.Assert(receipt.LastModified, qt.Equals, int64(0))
	})

	c.Run("Receipt", func(c *qt.C) {
		client := newTestClient(c, codecs.JSONCodec{})
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
		client := newTestClient(c, codecs.JSONCodec{}, "EXECRPC_NO_HASHER=true")
		result := runBasicTestForClient(c, client)
		assertMessages(c, result, 1)
		receipt := <-result.Receipt()
		c.Assert(receipt.ETag, qt.Equals, "")
	})

	c.Run("No reading Receipt", func(c *qt.C) {
		client := newTestClient(c, codecs.JSONCodec{}, "EXECRPC_NO_READING_RECEIPT=true")
		result := runBasicTestForClient(c, client)
		assertMessages(c, result, 1)
		receipt := <-result.Receipt()
		// Empty receipt.
		c.Assert(receipt.LastModified, qt.Equals, int64(0))
	})

	c.Run("No reading Receipt, no Close", func(c *qt.C) {
		client := newTestClient(c, codecs.JSONCodec{}, "EXECRPC_NO_READING_RECEIPT=true", "EXECRPC_NO_CLOSE=true")
		result := runBasicTestForClient(c, client)
		assertMessages(c, result, 1)
		receipt := <-result.Receipt()
		// Empty receipt.
		c.Assert(receipt.LastModified, qt.Equals, int64(0))
	})

	c.Run("Send log message from server", func(c *qt.C) {
		var logMessages []execrpc.Message

		client, err := execrpc.StartClient(
			execrpc.ClientOptions[model.ExampleRequest, model.ExampleMessage, model.ExampleReceipt]{
				ClientRawOptions: execrpc.ClientRawOptions{
					Version: 1,
					Cmd:     "go",
					Dir:     "./examples/servers/typed",
					Args:    []string{"run", "."},
					Env:     []string{"EXECRPC_SEND_TWO_LOG_MESSAGES=true"},
					Timeout: 30 * time.Second,
				},
				Codec: codecs.JSONCodec{},
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
		client := newTestClient(c, codecs.TOMLCodec{})
		result := runBasicTestForClient(c, client)
		assertMessages(c, result, 1)
	})

	c.Run("Error in receipt", func(c *qt.C) {
		client := newTestClient(c, codecs.JSONCodec{}, "EXECRPC_CALL_SHOULD_FAIL=true")
		result := client.Execute(model.ExampleRequest{Text: "hello"})
		c.Assert(result.Err(), qt.IsNil)
		receipt := <-result.Receipt()
		c.Assert(receipt.Error, qt.Not(qt.IsNil))
		assertMessages(c, result, 0)
	})

	// The "stdout print tests" are just to make sure that the server behaves and does not hang.

	c.Run("Print to stdout outside server before", func(c *qt.C) {
		client := newTestClient(c, codecs.JSONCodec{}, "EXECRPC_PRINT_OUTSIDE_SERVER_BEFORE=true")
		runBasicTestForClient(c, client)
	})

	c.Run("Print to stdout inside server", func(c *qt.C) {
		client := newTestClient(c, codecs.JSONCodec{}, "EXECRPC_PRINT_INSIDE_SERVER=true")
		runBasicTestForClient(c, client)
	})

	c.Run("Print to stdout outside server before", func(c *qt.C) {
		client := newTestClient(c, codecs.JSONCodec{}, "EXECRPC_PRINT_OUTSIDE_SERVER_BEFORE=true")
		runBasicTestForClient(c, client)
	})

	c.Run("Print to stdout inside after", func(c *qt.C) {
		client := newTestClient(c, codecs.JSONCodec{}, "EXECRPC_PRINT_OUTSIDE_SERVER_AFTER=true")
		runBasicTestForClient(c, client)
	})
}

func TestExecTypedConcurrent(t *testing.T) {
	client := newTestClient(t, codecs.JSONCodec{})
	var g errgroup.Group

	for i := 0; i < 100; i++ {
		i := i
		g.Go(func() error {
			for j := 0; j < 10; j++ {
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

	runBenchmark := func(name string, codec codecs.Codec, env ...string) {
		b.Run(name, func(b *testing.B) {
			client := newTestClient(b, codec, env...)
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

	runBenchmarksForCodec := func(codec codecs.Codec) {
		runBenchmark("1 message "+codec.Name(), codec)
		runBenchmark("100 messages "+codec.Name(), codec, "EXECRPC_NUM_MESSAGES=100")
	}
	runBenchmarksForCodec(codecs.JSONCodec{})
	runBenchmark("100 messages JSON, no hasher ", codecs.JSONCodec{}, "EXECRPC_NUM_MESSAGES=100", "EXECRPC_NO_HASHER=true")
	runBenchmarksForCodec(codecs.TOMLCodec{})
}
