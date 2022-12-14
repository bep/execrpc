package main

import (
	"fmt"
	"log"
	"os"

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
	)

	if printOutsideServerBefore {
		fmt.Println("Printing outside server before")
	}

	server, err := execrpc.NewServer(
		execrpc.ServerOptions[model.ExampleRequest, model.ExampleResponse]{
			Call: func(d execrpc.Dispatcher, req model.ExampleRequest) model.ExampleResponse {
				if printInsideServer {
					fmt.Println("Printing inside server")
				}
				if callShouldFail {
					return model.ExampleResponse{
						Error: &model.Error{Msg: "failed to echo"},
					}
				}

				if sendLogMessage {
					d.Send(
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
						})
				}

				return model.ExampleResponse{
					Hello: "Hello " + req.Text + "!",
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
