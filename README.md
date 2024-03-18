[![Tests on Linux, MacOS and Windows](https://github.com/bep/execrpc/workflows/Test/badge.svg)](https://github.com/bep/execrpc/actions?query=workflow:Test)
[![Go Report Card](https://goreportcard.com/badge/github.com/bep/execrpc)](https://goreportcard.com/report/github.com/bep/execrpc)
[![GoDoc](https://godoc.org/github.com/bep/execrpc?status.svg)](https://godoc.org/github.com/bep/execrpc)

This library implements a simple, custom [RPC protocol](https://en.wikipedia.org/wiki/Remote_procedure_call) via [os/exex](https://pkg.go.dev/os/exec) and stdin and stdout. Both server and client comes in a raw (`[]byte`) and strongly typed variant (using Go generics).

A strongly typed client may look like this:

```go
// Define the request, message and receipt types for the RPC calk,
client, err := execrpc.StartClient(
		execrpc.ClientOptions[model.ExampleRequest, model.ExampleMessage, model.ExampleReceipt]{
			ClientRawOptions: execrpc.ClientRawOptions{
				Version: 1,
				Cmd:     "go",
				Dir:     "./examples/servers/typed",
				Args:    []string{"run", "."},
				Env:     env,
				Timeout: 4 * time.Second,
			},
			Codec: codec,
		},
	)

if err != nil {
		logg.Fatal(err)
}


// Consume standalone messages (e.g. log messages) in its own goroutine.
go func() {
	for msg := range client.MessagesRaw() {
		fmt.Println("got message", string(msg.Body))
	}
}()

// Execute the request.
result := client.Execute(model.ExampleRequest{Text: "world"})

// Check for errors.
if  err; result.Err(); err != nil {
	logg.Fatal(err)
}

// Consume the messages.
for m := range result.Messages() {
	fmt.Println(m)
}

// Wait for the receipt.
receipt := result.Receipt()

// Check again for errors.
if  err; result.Err(); err != nil {
	logg.Fatal(err)
}

fmt.Println(receipt.Text)

```

To get the best performance you should keep the client open as long as its needed – and store it as a shared object; it's safe and encouraged to call `Execute` from multiple goroutines.

And the server side of the above:

```go
func main() {
	getHasher := func() hash.Hash {
		return fnv.New64a()
	}

	server, err := execrpc.NewServer(
		execrpc.ServerOptions[model.ExampleRequest, model.ExampleMessage, model.ExampleReceipt]{
			// Optional function to get a hasher for the ETag.
			GetHasher: getHasher,
			Handle: func(c *execrpc.Call[model.ExampleRequest, model.ExampleMessage, model.ExampleReceipt]) {
				// Raw messages are passed directly to the client,
				// typically used for log messages.
				c.SendRaw(
					execrpc.Message{
						Header: execrpc.Header{
							Version: 32,
							Status:  150,
						},
						Body: []byte("a log message"),
					},
				)


				// Send one or more messages.
				c.Send(
					model.ExampleMessage{
						Hello: "Hello 1!",
					},
				)

				c.Send(
					model.ExampleMessage{
						Hello: "Hello 2!",
					},
				)

				// Close the message stream and optionally wait for a receipt.
				receipt := <-c.Close(
					model.ExampleReceipt{
						Text: "echoed: " + c.Request.Text,
						Identity: execrpc.Identity{
							// Just set size, the rest will be filled in by RPC library.
							Size: 123,
						},
					},
				)

				// ETag provided by the library.
				// A hash of all message bodies.
				fmt.Println("Receipt:", receipt.ETag)
			},
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	_ = server.Wait()
}

```

## Status Codes

The status codes in the header between 1 and 99 are reserved for the system. This will typically be used to catch decoding/encoding errors on the server.