package main

import (
	"os"

	"github.com/bep/execrpc"
)

func main() {
	server, err := execrpc.NewServerRaw(
		execrpc.ServerRawOptions{
			In:  os.Stdin,
			Out: os.Stdout,
			Call: func(message execrpc.Message) execrpc.Message {
				return execrpc.Message{
					Header: message.Header,
					Body:   append([]byte("echo: "), message.Body...),
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
	print("error: failed to start echo server:", err)
	os.Exit(1)
}
