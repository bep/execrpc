package main

import (
	"fmt"
	"os"

	"github.com/bep/execrpc"
)

func main() {
	server := execrpc.NewServer(
		execrpc.ServerOptions{
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

	if err := server.Start(); err != nil {
		print("error: failed to start echo server:", err)
		os.Exit(1)
	}
	_ = server.Wait()
}

func print(s string, args ...any) {
	fmt.Fprintln(os.Stderr, s, args)
}
