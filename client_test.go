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
	client, err := execrpc.StartClientRaw(
		execrpc.ClientRawOptions{
			Version: 1,
			Cmd:     "go",
			Args:    []string{"run", "./examples/servers/raw"},
		})

	c.Assert(err, qt.IsNil)

	defer func() {
		c.Assert(client.Close(), qt.IsNil)
	}()

	c.Run("OK", func(c *qt.C) {
		result, err := client.Execute([]byte("hello"))
		c.Assert(err, qt.IsNil)
		c.Assert(string(result.Body), qt.Equals, "echo: hello")
	})

}

func newTestClient(t testing.TB, codec codecs.Codec[model.ExampleRequest, model.ExampleResponse], env ...string) *execrpc.Client[model.ExampleRequest, model.ExampleResponse] {
	client, err := execrpc.StartClient(
		execrpc.ClientOptions[model.ExampleRequest, model.ExampleResponse]{
			ClientRawOptions: execrpc.ClientRawOptions{
				Version: 1,
				Cmd:     "go",
				Args:    []string{"run", "./examples/servers/typed"},
				Env:     env,
			},
			Codec: codec,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestExecTyped(t *testing.T) {
	c := qt.New(t)

	newClient := func(t testing.TB, codec codecs.Codec[model.ExampleRequest, model.ExampleResponse], env ...string) *execrpc.Client[model.ExampleRequest, model.ExampleResponse] {
		client, err := execrpc.StartClient(
			execrpc.ClientOptions[model.ExampleRequest, model.ExampleResponse]{
				ClientRawOptions: execrpc.ClientRawOptions{
					Version: 1,
					Cmd:     "go",
					Args:    []string{"run", "./examples/servers/typed"},
					Env:     env,
					Timeout: 4 * time.Second,
				},
				Codec: codec,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		return client
	}

	runBasicTestForClient := func(c *qt.C, client *execrpc.Client[model.ExampleRequest, model.ExampleResponse]) model.ExampleResponse {
		result, err := client.Execute(model.ExampleRequest{Text: "world"})
		c.Assert(err, qt.IsNil)
		c.Assert(result.Err(), qt.IsNil)
		c.Assert(string(result.Hello), qt.Equals, "Hello world!")
		c.Assert(client.Close(), qt.IsNil)
		return result

	}

	c.Run("JSON", func(c *qt.C) {
		client := newClient(c, codecs.JSONCodec[model.ExampleRequest, model.ExampleResponse]{}, "EXECRPC_CODEC=json")
		runBasicTestForClient(c, client)
	})

	c.Run("TOML", func(c *qt.C) {
		client := newClient(c, codecs.TOMLCodec[model.ExampleRequest, model.ExampleResponse]{}, "EXECRPC_CODEC=toml")
		runBasicTestForClient(c, client)
	})

	c.Run("Gob", func(c *qt.C) {
		client := newClient(c, codecs.GobCodec[model.ExampleRequest, model.ExampleResponse]{}, "EXECRPC_CODEC=gob")
		runBasicTestForClient(c, client)
	})

	c.Run("Send log message from server", func(c *qt.C) {
		client := newClient(c, codecs.JSONCodec[model.ExampleRequest, model.ExampleResponse]{}, "EXECRPC_CODEC=json", "EXECRPC_SEND_LOG_MESSAGE=true")
		result, err := client.Execute(model.ExampleRequest{Text: "world"})
		c.Assert(err, qt.IsNil)
		c.Assert(result.Err(), qt.IsNil)
		c.Assert(string(result.Hello), qt.Equals, "Hello world!")
		c.Assert(client.Close(), qt.IsNil)
	})

	c.Run("Error", func(c *qt.C) {
		client := newClient(c, codecs.JSONCodec[model.ExampleRequest, model.ExampleResponse]{}, "EXECRPC_CODEC=json", "EXECRPC_CALL_SHOULD_FAIL=true")
		result, err := client.Execute(model.ExampleRequest{Text: "hello"})
		c.Assert(err, qt.IsNil)
		c.Assert(result.Err(), qt.IsNotNil)
		c.Assert(client.Close(), qt.IsNil)
	})

	// The "stdout print tests" are just to make sure that the server behaves and does not hang.

	c.Run("Print to stdout outside server before", func(c *qt.C) {
		client := newClient(c, codecs.JSONCodec[model.ExampleRequest, model.ExampleResponse]{}, "EXECRPC_PRINT_OUTSIDE_SERVER_BEFORE=true")
		runBasicTestForClient(c, client)
	})

	c.Run("Print to stdout inside server", func(c *qt.C) {
		client := newClient(c, codecs.JSONCodec[model.ExampleRequest, model.ExampleResponse]{}, "EXECRPC_PRINT_INSIDE_SERVER=true")
		runBasicTestForClient(c, client)
	})

	c.Run("Print to stdout outside server before", func(c *qt.C) {
		client := newClient(c, codecs.JSONCodec[model.ExampleRequest, model.ExampleResponse]{}, "EXECRPC_PRINT_OUTSIDE_SERVER_BEFORE=true")
		runBasicTestForClient(c, client)
	})

	c.Run("Print to stdout inside after", func(c *qt.C) {
		client := newClient(c, codecs.JSONCodec[model.ExampleRequest, model.ExampleResponse]{}, "EXECRPC_PRINT_OUTSIDE_SERVER_AFTER=true")
		runBasicTestForClient(c, client)
	})

}

func TestExecTypedConcurrent(t *testing.T) {
	client := newTestClient(t, codecs.JSONCodec[model.ExampleRequest, model.ExampleResponse]{}, "EXECRPC_CODEC=json")
	var g errgroup.Group

	for i := 0; i < 100; i++ {
		i := i
		g.Go(func() error {
			for j := 0; j < 10; j++ {
				text := fmt.Sprintf("%d-%d", i, j)
				result, err := client.Execute(model.ExampleRequest{Text: text})
				if err != nil {
					return err
				}
				if result.Err() != nil {
					return result.Err()
				}
				expect := fmt.Sprintf("Hello %s!", text)
				if string(result.Hello) != expect {
					return fmt.Errorf("unexpected result: %s", result.Hello)
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

	b.Run("JSON", func(b *testing.B) {
		client := newTestClient(b, codecs.JSONCodec[model.ExampleRequest, model.ExampleResponse]{}, "EXECRPC_CODEC=json")
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_, err := client.Execute(model.ExampleRequest{Text: word})
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	})

	b.Run("TOML", func(b *testing.B) {
		client := newTestClient(b, codecs.TOMLCodec[model.ExampleRequest, model.ExampleResponse]{}, "EXECRPC_CODEC=toml")
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_, err := client.Execute(model.ExampleRequest{Text: word})
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	})

	b.Run("Gob", func(b *testing.B) {
		client := newTestClient(b, codecs.GobCodec[model.ExampleRequest, model.ExampleResponse]{}, "EXECRPC_CODEC=gob")
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_, err := client.Execute(model.ExampleRequest{Text: word})
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	})

}
