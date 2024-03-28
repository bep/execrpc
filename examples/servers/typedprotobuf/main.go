package main

import (
	"fmt"
	"hash"
	"hash/fnv"
	"log"
	"os"
	"strconv"

	"github.com/bep/execrpc"
	"github.com/bep/execrpc/examples/modelprotobuf"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("typed-protobuf-example: ")

	// Some test flags from the client.
	var (
		printOutsideServerBefore = os.Getenv("EXECRPC_PRINT_OUTSIDE_SERVER_BEFORE") != ""
		printOutsideServerAfter  = os.Getenv("EXECRPC_PRINT_OUTSIDE_SERVER_AFTER") != ""
		printInsideServer        = os.Getenv("EXECRPC_PRINT_INSIDE_SERVER") != ""
		callShouldFail           = os.Getenv("EXECRPC_CALL_SHOULD_FAIL") != ""
		sendLogMessage           = os.Getenv("EXECRPC_SEND_TWO_LOG_MESSAGES") != ""
		noClose                  = os.Getenv("EXECRPC_NO_CLOSE") != ""
		numMessagesStr           = os.Getenv("EXECRPC_NUM_MESSAGES")
		noHasher                 = os.Getenv("EXECRPC_NO_HASHER") != ""
		numMessages              = 1
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
		execrpc.ServerOptions[*modelprotobuf.ExampleRequest, *modelprotobuf.ExampleMessage, *modelprotobuf.ExampleReceipt]{
			GetHasher: getHasher,
			Handle: func(c *execrpc.Call[*modelprotobuf.ExampleRequest, *modelprotobuf.ExampleMessage, *modelprotobuf.ExampleReceipt]) {
				if printInsideServer {
					fmt.Println("Printing inside server")
				}
				if callShouldFail {
					c.Close(
						&modelprotobuf.ExampleReceipt{
							Error: &modelprotobuf.Error{Msg: "failed to echo"},
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
					c.Send(
						&modelprotobuf.ExampleMessage{
							Hello: strconv.Itoa(i) + ": Hello " + c.Request.Text + "!",
						},
					)
				}

				if !noClose {
					receipt := <-c.Close(
						&modelprotobuf.ExampleReceipt{
							Text: "echoed: " + c.Request.Text,
							Identity: &modelprotobuf.Identity{
								// Just set size, the rest will be filled in by RPC library.
								Size: 123,
							},
						},
					)
					if receipt.Text != "echoed: "+c.Request.Text {
						log.Fatalf("expected receipt text to be %q, got %q", "echoed: "+c.Request.Text, receipt.Text)
					}
					if receipt.Identity.ETag == "" {
						log.Fatalf("expected receipt eTag to be set")
					}
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
	_ = server.Wait()
}

func handleErr(err error) {
	log.Fatalf("error: failed to start typed echo server: %s", err)
}
