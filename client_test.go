package execrpc_test

import (
	"testing"

	"github.com/bep/execrpc"
	"github.com/bep/execrpc/codecs"
	"github.com/bep/execrpc/examples/model"
	qt "github.com/frankban/quicktest"
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

func TestExecTyped(t *testing.T) {
	c := qt.New(t)

	newCient := func(t testing.TB, codecID string, codec codecs.Codec[model.ExampleRequest, model.ExampleResponse]) *execrpc.Client[model.ExampleRequest, model.ExampleResponse] {
		client, err := execrpc.StartClient(
			execrpc.ClientOptions[model.ExampleRequest, model.ExampleResponse]{
				ClientRawOptions: execrpc.ClientRawOptions{
					Version: 1,
					Cmd:     "go",
					Args:    []string{"run", "./examples/servers/typed"},
					Env:     []string{"EXECRPC_CODEC=" + codecID},
				},
				Codec: codec,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		return client
	}

	c.Run("JSON", func(c *qt.C) {
		client := newCient(c, "json", codecs.JSONCodec[model.ExampleRequest, model.ExampleResponse]{})
		result, err := client.Execute(model.ExampleRequest{Text: "world"})
		c.Assert(err, qt.IsNil)
		c.Assert(result.Err(), qt.IsNil)
		c.Assert(string(result.Hello), qt.Equals, "Hello world!")
		c.Assert(client.Close(), qt.IsNil)
	})

	c.Run("TOML", func(c *qt.C) {
		client := newCient(c, "toml", codecs.TOMLCodec[model.ExampleRequest, model.ExampleResponse]{})
		result, err := client.Execute(model.ExampleRequest{Text: "world"})
		c.Assert(err, qt.IsNil)
		c.Assert(result.Err(), qt.IsNil)
		c.Assert(string(result.Hello), qt.Equals, "Hello world!")
		c.Assert(client.Close(), qt.IsNil)
	})

	c.Run("Gob", func(c *qt.C) {
		client := newCient(c, "gob", codecs.GobCodec[model.ExampleRequest, model.ExampleResponse]{})
		result, err := client.Execute(model.ExampleRequest{Text: "world"})
		c.Assert(err, qt.IsNil)
		c.Assert(result.Err(), qt.IsNil)
		c.Assert(string(result.Hello), qt.Equals, "Hello world!")
		c.Assert(client.Close(), qt.IsNil)
	})

	c.Run("Error", func(c *qt.C) {
		client := newCient(c, "json", codecs.JSONCodec[model.ExampleRequest, model.ExampleResponse]{})
		result, err := client.Execute(model.ExampleRequest{Text: "fail"})
		c.Assert(err, qt.IsNil)
		c.Assert(result.Err(), qt.IsNotNil)
		c.Assert(client.Close(), qt.IsNil)
	})

}

func BenchmarkClient(b *testing.B) {
	newCient := func(t testing.TB, codecID string, codec codecs.Codec[model.ExampleRequest, model.ExampleResponse]) *execrpc.Client[model.ExampleRequest, model.ExampleResponse] {
		client, err := execrpc.StartClient(
			execrpc.ClientOptions[model.ExampleRequest, model.ExampleResponse]{
				ClientRawOptions: execrpc.ClientRawOptions{
					Version: 1,
					Cmd:     "go",
					Args:    []string{"run", "./examples/servers/typed"},
					Env:     []string{"EXECRPC_CODEC=" + codecID},
				},
				Codec: codec,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		return client
	}

	const word = "World"

	b.Run("JSON", func(b *testing.B) {
		client := newCient(b, "json", codecs.JSONCodec[model.ExampleRequest, model.ExampleResponse]{})
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
		client := newCient(b, "toml", codecs.TOMLCodec[model.ExampleRequest, model.ExampleResponse]{})
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
		client := newCient(b, "gob", codecs.GobCodec[model.ExampleRequest, model.ExampleResponse]{})
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
