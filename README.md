[![Tests on Linux, MacOS and Windows](https://github.com/bep/execrpc/workflows/Test/badge.svg)](https://github.com/bep/execrpc/actions?query=workflow:Test)
[![Go Report Card](https://goreportcard.com/badge/github.com/bep/execrpc)](https://goreportcard.com/report/github.com/bep/execrpc)
[![GoDoc](https://godoc.org/github.com/bep/execrpc?status.svg)](https://godoc.org/github.com/bep/execrpc)

This library implements a simple, custom [RPC protocol](https://en.wikipedia.org/wiki/Remote_procedure_call) via [os/exex](https://pkg.go.dev/os/exec) and stdin and stdout. Both server and client comes in a raw (`[]byte`) and strongly typed variant (using Go generics).

A strongly typed client may look like this:

```go
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

result, _ := client.Execute(model.ExampleRequest{Text: "world"})

fmt.Println(result.Hello)
```

And the server side of the above:

```go
func main() {
	server, _ := execrpc.NewServer(
		execrpc.ServerOptions[model.ExampleRequest, model.ExampleResponse]{
			Codec: codecs.JSONCodec[model.ExampleResponse, model.ExampleRequest]{},
			Call: func(req model.ExampleRequest) model.ExampleResponse {
				return model.ExampleResponse{
					Hello: "Hello " + req.Text + "!",
				}
			},
		},
	)
	if err := server.Start(); err != nil {
		// ... handle error
	}
	_ = server.Wait()
}
```
