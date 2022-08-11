package main

import (
	"log"
	"os"

	"github.com/bep/execrpc"
	"github.com/bep/execrpc/codecs"
	"github.com/bep/execrpc/examples/model"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("typed-example: ")

	codecID := os.Getenv("EXECRPC_CODEC")
	if codecID == "" {
		codecID = "json"
	}

	// Note that it's not possible to print anything to stdout.
	// Use stdout for logging.
	log.Printf("Starting server using codec %s", codecID)

	var codec codecs.Codec[model.ExampleResponse, model.ExampleRequest]
	switch codecID {
	case "toml":
		codec = codecs.TOMLCodec[model.ExampleResponse, model.ExampleRequest]{}
	case "gob":
		codec = codecs.GobCodec[model.ExampleResponse, model.ExampleRequest]{}
	default:
		codec = codecs.JSONCodec[model.ExampleResponse, model.ExampleRequest]{}
	}

	server, err := execrpc.NewServer(
		execrpc.ServerOptions[model.ExampleRequest, model.ExampleResponse]{
			Codec: codec,
			Call: func(req model.ExampleRequest) model.ExampleResponse {
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
