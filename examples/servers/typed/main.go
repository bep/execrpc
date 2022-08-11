package main

import (
	"fmt"
	"log"
	"os"

	"github.com/bep/execrpc"
	"github.com/bep/execrpc/codecs"
	"github.com/bep/execrpc/examples/model"
)

func main() {
	server, err := execrpc.NewServer(
		execrpc.ServerOptions[model.ExampleRequest, model.ExampleResponse]{
			Codec: codecs.JSONCodec[model.ExampleResponse, model.ExampleRequest]{},
			Call: func(req model.ExampleRequest) model.ExampleResponse {
				if req.Text == "stdout" {
					// Make sure that the server doesn't hang when writing to os.Stdout.
					fmt.Fprintln(os.Stdout, "write some text to stdout, this should end up in stderr")
				}

				if req.Text == "fail" {
					return model.ExampleResponse{
						Error: &model.Error{Msg: "failed to echo"},
					}
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
	_ = server.Wait()
}

func handleErr(err error) {
	log.Fatalf("error: failed to start typed echo server: %s", err)

}
