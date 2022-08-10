package execrpc

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestExec(t *testing.T) {
	c := qt.New(t)
	client, err := StartClient(
		ClientOptions{
			Version: 1,
			Cmd:     "go",
			Args:    []string{"run", "./examples/server"},
		})
	defer func() {
		c.Assert(client.Close(), qt.IsNil)
	}()
	c.Assert(err, qt.IsNil)
	result, err := client.Execute([]byte("hello"))
	c.Assert(err, qt.IsNil)
	c.Assert(string(result.Body), qt.Equals, "echo: hello")

}
