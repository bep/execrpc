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

//...

client.Close()

```

To get the best performance you should keep the client open as long as its needed – and store it as a shared object; it's safe and encouraged to call `Execute` from multiple goroutines.

And the server side of the above:

```go
func main() {
	server, _ := execrpc.NewServer(
		execrpc.ServerOptions[model.ExampleRequest, model.ExampleResponse]{
			Codec: codecs.JSONCodec[model.ExampleResponse, model.ExampleRequest]{},
			Call: func(d execrpc.Dispatcher, req model.ExampleRequest) model.ExampleResponse {
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

Of the included codecs, JSON seems to win by a small margin (but only tested with small requests/responses):

```bsh
name            time/op
Client/JSON-10  4.89µs ± 0%
Client/TOML-10  5.51µs ± 0%
Client/Gob-10   17.0µs ± 0%

name            alloc/op
Client/JSON-10    922B ± 0%
Client/TOML-10  1.67kB ± 0%
Client/Gob-10   9.22kB ± 0%

name            allocs/op
Client/JSON-10    19.0 ± 0%
Client/TOML-10    28.0 ± 0%
Client/Gob-10      227 ± 0%
```

## Status Codes

The status codes in the header between 1 and 99 are reserved for the system. This will typically be used to catch decoding/encoding errors on the server.