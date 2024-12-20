package main

import (
	"fmt"
	"hash"
	"hash/fnv"
	"log"

	"github.com/bep/execrpc"
	"github.com/bep/execrpc/examples/model"
)

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
			Init: func(cfg model.ExampleConfig, procol execrpc.ProtocolInfo) error {
				if procol.Version != 3 {
					return fmt.Errorf("unsupported protocol version: %d", procol.Version)
				}
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
