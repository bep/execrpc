package model

import "github.com/bep/execrpc"

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
