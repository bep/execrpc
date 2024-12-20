package main

import (
	"fmt"
	"hash"
	"hash/fnv"
	"log"
	"os"
	"strconv"

	"github.com/bep/execrpc"
	"github.com/bep/execrpc/examples/model"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("typed-example: ")

	// Some test flags from the client.
	var (
		delayDelivery            = os.Getenv("EXECRPC_DELAY_DELIVERY") != ""
		noHasher                 = os.Getenv("EXECRPC_NO_HASHER") != ""
		printOutsideServerBefore = os.Getenv("EXECRPC_PRINT_OUTSIDE_SERVER_BEFORE") != ""
		printOutsideServerAfter  = os.Getenv("EXECRPC_PRINT_OUTSIDE_SERVER_AFTER") != ""
		printInsideServer        = os.Getenv("EXECRPC_PRINT_INSIDE_SERVER") != ""
	)

	if printOutsideServerBefore {
		fmt.Println("Printing outside server before")
	}

	var getHasher func() hash.Hash

	if !noHasher {
		getHasher = func() hash.Hash {
			return fnv.New64a()
		}
	}

	var clientConfig model.ExampleConfig

	server, err := execrpc.NewServer(
		execrpc.ServerOptions[model.ExampleConfig, model.ExampleRequest, model.ExampleMessage, model.ExampleReceipt]{
			GetHasher:     getHasher,
			DelayDelivery: delayDelivery,
			Init: func(cfg model.ExampleConfig, protocol execrpc.ProtocolInfo) error {
				if protocol.Version != 3 {
					return fmt.Errorf("unsupported protocol version: %d", protocol.Version)
				}
				clientConfig = cfg
				return clientConfig.Init()
			},
			Handle: func(call *execrpc.Call[model.ExampleRequest, model.ExampleMessage, model.ExampleReceipt]) {
				if printInsideServer {
					fmt.Println("Printing inside server")
				}
				if clientConfig.CallShouldFail {
					call.Close(
						false,
						model.ExampleReceipt{
							Error: &model.Error{Msg: "failed to echo"},
						},
					)
					return
				}

				if clientConfig.SendLogMessage {
					call.SendRaw(
						execrpc.Message{
							Header: execrpc.Header{
								Version: 32,
								Status:  150,
							},
							Body: []byte("first log message"),
						},
						execrpc.Message{
							Header: execrpc.Header{
								Version: 32,
								Status:  150,
							},
							Body: []byte("second log message"),
						},
					)
				}

				for i := 0; i < clientConfig.NumMessages; i++ {
					call.Enqueue(
						model.ExampleMessage{
							Hello: strconv.Itoa(i) + ": Hello " + call.Request.Text + "!",
						},
					)
				}

				if !clientConfig.NoClose {
					var receipt model.ExampleReceipt
					if !clientConfig.NoReadingReceipt {
						receipt = <-call.Receipt()
						receipt.Text = "echoed: " + call.Request.Text
						receipt.Size = uint32(123)
					}

					call.Close(clientConfig.DropMessages, receipt)
				}
			},
		},
	)
	if err != nil {
		handleErr(err)
	}

	if err := server.Start(); err != nil {
		handleErr(err)
	}

	if printOutsideServerAfter {
		fmt.Println("Printing outside server after")
	}
}

func handleErr(err error) {
	log.Fatalf("error: failed to start typed echo server: %s", err)
}
