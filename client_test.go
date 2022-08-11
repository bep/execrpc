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
	client, err := execrpc.StartClient(
		execrpc.ClientOptions[model.ExampleRequest, model.ExampleResponse]{
			ClientRawOptions: execrpc.ClientRawOptions{
				Version: 1,
				Cmd:     "go",
				Args:    []string{"run", "./examples/servers/typed"},
			},
			Codec: codecs.JSONCodec[model.ExampleRequest, model.ExampleResponse]{},
		},
	)

	defer func() {
		c.Assert(client.Close(), qt.IsNil)
	}()

	c.Run("OK", func(c *qt.C) {
		c.Assert(err, qt.IsNil)
		result, err := client.Execute(model.ExampleRequest{Text: "world"})
		c.Assert(err, qt.IsNil)
		c.Assert(result.Err(), qt.IsNil)
		c.Assert(string(result.Hello), qt.Equals, "Hello world!")
	})

	c.Run("Error", func(c *qt.C) {
		c.Assert(err, qt.IsNil)
		result, err := client.Execute(model.ExampleRequest{Text: "fail"})
		c.Assert(err, qt.IsNil)
		c.Assert(result.Err(), qt.IsNotNil)
	})

	c.Run("Server writes to os.Stdout", func(c *qt.C) {
		c.Assert(err, qt.IsNil)
		// Signal to the server that it should also be written to os.Stdout.
		result, err := client.Execute(model.ExampleRequest{Text: "stdout"})
		c.Assert(err, qt.IsNil)
		c.Assert(result.Err(), qt.IsNil)
		c.Assert(string(result.Hello), qt.Equals, "Hello stdout!")
	})

}
