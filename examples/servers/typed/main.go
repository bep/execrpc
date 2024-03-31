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
		printOutsideServerBefore = os.Getenv("EXECRPC_PRINT_OUTSIDE_SERVER_BEFORE") != ""
		printOutsideServerAfter  = os.Getenv("EXECRPC_PRINT_OUTSIDE_SERVER_AFTER") != ""
		printInsideServer        = os.Getenv("EXECRPC_PRINT_INSIDE_SERVER") != ""
		callShouldFail           = os.Getenv("EXECRPC_CALL_SHOULD_FAIL") != ""
		sendLogMessage           = os.Getenv("EXECRPC_SEND_TWO_LOG_MESSAGES") != ""
		noClose                  = os.Getenv("EXECRPC_NO_CLOSE") != ""
		noReadingReceipt         = os.Getenv("EXECRPC_NO_READING_RECEIPT") != ""
		numMessagesStr           = os.Getenv("EXECRPC_NUM_MESSAGES")
		numMessages              = 1
		delayDelivery            = os.Getenv("EXECRPC_DELAY_DELIVERY") != ""
		dropMessages             = os.Getenv("EXECRPC_DROP_MESSAGES") != ""
		noHasher                 = os.Getenv("EXECRPC_NO_HASHER") != ""
	)

	if numMessagesStr != "" {
		numMessages, _ = strconv.Atoi(numMessagesStr)
		if numMessages < 1 {
			numMessages = 1
		}
	}

	if printOutsideServerBefore {
		fmt.Println("Printing outside server before")
	}

	var getHasher func() hash.Hash

	if !noHasher {
		getHasher = func() hash.Hash {
			return fnv.New64a()
		}
	}

	server, err := execrpc.NewServer(
		execrpc.ServerOptions[model.ExampleRequest, model.ExampleMessage, model.ExampleReceipt]{
			GetHasher:     getHasher,
			DelayDelivery: delayDelivery,
			Handle: func(c *execrpc.Call[model.ExampleRequest, model.ExampleMessage, model.ExampleReceipt]) {
				if printInsideServer {
					fmt.Println("Printing inside server")
				}
				if callShouldFail {
					c.Close(
						false,
						model.ExampleReceipt{
							Error: &model.Error{Msg: "failed to echo"},
						},
					)
					return
				}

				if sendLogMessage {
					c.SendRaw(
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

				for i := 0; i < numMessages; i++ {
					c.Enqueue(
						model.ExampleMessage{
							Hello: strconv.Itoa(i) + ": Hello " + c.Request.Text + "!",
						},
					)
				}

				if !noClose {
					var receipt model.ExampleReceipt
					if !noReadingReceipt {
						receipt = <-c.Receipt()
						receipt.Text = "echoed: " + c.Request.Text
						receipt.Size = uint32(123)
					}

					c.Close(dropMessages, receipt)
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
