// Copyright 2025 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package main

import (
	"os"

	"github.com/bep/execrpc"
)

func main() {
	server, err := execrpc.NewServerRaw(
		execrpc.ServerRawOptions{
			Call: func(req execrpc.Message, d execrpc.Dispatcher) error {
				header := req.Header
				// execrpc.MessageStatusOK will complete the exchange.
				// Setting it to execrpc.MessageStatusContinue will continue the conversation.
				header.Status = execrpc.MessageStatusOK
				d.SendMessage(
					execrpc.Message{
						Header: header,
						Body:   append([]byte("echo: "), req.Body...),
					},
				)
				return nil
			},
		},
	)
	if err != nil {
		handleErr(err)
	}

	if err := server.Start(); err != nil {
		handleErr(err)
	}
}

func handleErr(err error) {
	print("error: failed to start echo server:", err)
	os.Exit(1)
}
