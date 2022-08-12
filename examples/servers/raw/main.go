package main

import (
	"os"

	"github.com/bep/execrpc"
)

func main() {
	server, err := execrpc.NewServerRaw(
		execrpc.ServerRawOptions{
			Call: func(d execrpc.Dispatcher, message execrpc.Message) (execrpc.Message, error) {
				return execrpc.Message{
					Header: message.Header,
					Body:   append([]byte("echo: "), message.Body...),
				}, nil
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
	print("error: failed to start echo server:", err)
	os.Exit(1)
}
