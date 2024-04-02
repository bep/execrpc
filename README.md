[![Tests on Linux, MacOS and Windows](https://github.com/bep/execrpc/workflows/Test/badge.svg)](https://github.com/bep/execrpc/actions?query=workflow:Test)
[![Go Report Card](https://goreportcard.com/badge/github.com/bep/execrpc)](https://goreportcard.com/report/github.com/bep/execrpc)
[![GoDoc](https://godoc.org/github.com/bep/execrpc?status.svg)](https://godoc.org/github.com/bep/execrpc)

This library implements a simple, custom [RPC protocol](https://en.wikipedia.org/wiki/Remote_procedure_call) via [os/exex](https://pkg.go.dev/os/exec) and stdin and stdout. Both server and client comes in a raw (`[]byte`) and strongly typed variant (using Go generics).

A strongly typed client may look like this:

```go
// Define the request, message and receipt types for the RPC call.
client, err := execrpc.StartClient(
	client, err := execrpc.StartClient(
	execrpc.ClientOptions[model.ExampleConfig, model.ExampleRequest, model.ExampleMessage, model.ExampleReceipt]{
		ClientRawOptions: execrpc.ClientRawOptions{
			Version: 1,
			Cmd:     "go",
			Dir:     "./examples/servers/typed",
			Args:    []string{"run", "."},
			Env:     env,
			Timeout: 30 * time.Second,
		},
		Config: model.ExampleConfig{},
		Codec:  codec,
	},
)

if err != nil {
	log.Fatal(err)
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
if err := result.Err(); err != nil {
	log.Fatal(err)
}

// Consume the messages.
for m := range result.Messages() {
	fmt.Println(m)
}

// Wait for the receipt.
receipt := <-result.Receipt()

// Check again for errors.
if err := result.Err(); err != nil {
	log.Fatal(err)
}

fmt.Println(receipt.Text)

// Close the client.
if err := client.Close(); err != nil {
	log.Fatal(err)
}
```

To get the best performance you should keep the client open as long as its needed – and store it as a shared object; it's safe and encouraged to call `Execute` from multiple goroutines.

And the server side of the above:

```go

func main() {
	log.SetFlags(0)
	log.SetPrefix("readme-example: ")

	var clientConfig model.ExampleConfig

	server, err := execrpc.NewServer(
		execrpc.ServerOptions[model.ExampleConfig, model.ExampleRequest, model.ExampleMessage, model.ExampleReceipt]{
			// Optional function to provide a hasher for the ETag.
			GetHasher: func() hash.Hash {
				return fnv.New64a()
			},

			// Allows you to delay message delivery, and drop
			// them after reading the receipt (e.g. the ETag matches the ETag seen by client).
			DelayDelivery: false,

			// Optional function to initialize the server
			// with the client configuration.
			// This will be called once on server start.
			Init: func(cfg model.ExampleConfig) error {
				clientConfig = cfg
				return clientConfig.Init()
			},

			// Handle the incoming call.
			Handle: func(c *execrpc.Call[model.ExampleRequest, model.ExampleMessage, model.ExampleReceipt]) {
				// Raw messages are passed directly to the client,
				// typically used for log messages.
				c.SendRaw(
					execrpc.Message{
						Header: execrpc.Header{
							Version: 32,
							Status:  150,
						},
						Body: []byte("log message"),
					},
				)

				// Enqueue one or more messages.
				c.Enqueue(
					model.ExampleMessage{
						Hello: "Hello 1!",
					},
					model.ExampleMessage{
						Hello: "Hello 2!",
					},
				)

				c.Enqueue(
					model.ExampleMessage{
						Hello: "Hello 3!",
					},
				)

				// Wait for the framework generated receipt.
				receipt := <-c.Receipt()

				// ETag provided by the framework.
				// A hash of all message bodies.
				// fmt.Println("Receipt:", receipt.ETag)

				// Modify if needed.
				receipt.Size = uint32(123)
				receipt.Text = "echoed: " + c.Request.Text

				// Close the message stream and send the receipt.
				// Pass true to drop any queued messages,
				// this is only relevant if DelayDelivery is enabled.
				c.Close(false, receipt)
			},
		},
	)
	if err != nil {
		handleErr(err)
	}

	if err := server.Start(); err != nil {
		handleErr(err)
	}
}

func handleErr(err error) {
	log.Fatalf("error: failed to start typed echo server: %s", err)
}

```

## Generate ETag

The server can generate an ETag for the messages. This is a hash of all message bodies. 

To enable this:

1. Provide a `GetHasher` function to the [server options](https://pkg.go.dev/github.com/bep/execrpc#ServerOptions).
2. Have the `Receipt` implement the [TagProvider](https://pkg.go.dev/github.com/bep/execrpc#TagProvider) interface.

Note that there are three different optional E-interfaces for the `Receipt`:

1. [TagProvider](https://pkg.go.dev/github.com/bep/execrpc#TagProvider) for the ETag.
2. [SizeProvider](https://pkg.go.dev/github.com/bep/execrpc#SizeProvider) for the size.
3. [LastModifiedProvider](https://pkg.go.dev/github.com/bep/execrpc#LastModifiedProvider) for the last modified timestamp.

A convenient struct that can be embedded in your `Receipt` that implements all of these is the [Identity](https://pkg.go.dev/github.com/bep/execrpc#Identity).

## Status Codes

The status codes in the header between 1 and 99 are reserved for the system. This will typically be used to catch decoding/encoding errors on the server.