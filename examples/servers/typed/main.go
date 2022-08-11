package main

import (
	"log"
	"os"

	"github.com/bep/execrpc"
	"github.com/bep/execrpc/codecs"
	"github.com/bep/execrpc/examples/model"
)

func main() {
	server, err := execrpc.NewServer(
		execrpc.ServerOptions[model.ExampleRequest, model.ExampelResponse]{
			In:    os.Stdin,
			Out:   os.Stdout,
			Codec: codecs.JSONCodec[model.ExampelResponse, model.ExampleRequest]{},
			Call: func(req model.ExampleRequest) model.ExampelResponse {
				if req.Text == "fail" {
					return model.ExampelResponse{
						Error: &model.Error{Msg: "failed to echo"},
					}
				}
				return model.ExampelResponse{
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
