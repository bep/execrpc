// Copyright 2025 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package model

import "github.com/bep/execrpc"

type ExampleConfig struct {
	// Used in tests.
	CallShouldFail   bool `json:"callShouldFail"`
	SendLogMessage   bool `json:"sendLogMessage"`
	NoClose          bool `json:"noClose"`
	NoReadingReceipt bool `json:"noReadingReceipt"`
	DropMessages     bool `json:"dropMessages"`
	NumMessages      int  `json:"numMessages"`
}

func (cfg *ExampleConfig) Init() error {
	if cfg.NumMessages < 1 {
		cfg.NumMessages = 1
	}

	return nil
}

// ExampleRequest is just a simple example request.
type ExampleRequest struct {
	Text string `json:"text"`
}

// ExampleMessage is just a simple example message.
type ExampleMessage struct {
	Hello string `json:"hello"`
}

// ExampleReceipt is just a simple receipt.
type ExampleReceipt struct {
	execrpc.Identity
	Error *Error `json:"err"`
	Text  string `json:"text"`
}

// Err is just a simple example error.
func (r ExampleReceipt) Err() error {
	if r.Error == nil {
		// Make sure that resp.Err() == nil.
		return nil
	}
	return r.Error
}

// Error holds an error message.
type Error struct {
	Msg string `json:"msg"`
}

func (r Error) Error() string {
	return r.Msg
}
