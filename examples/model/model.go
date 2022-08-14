package model

// ExampleRequest is just a simple example request.
type ExampleRequest struct {
	Text string `json:"text"`
}

// ExampleResponse is just a simple example response.
type ExampleResponse struct {
	Hello string `json:"hello"`
	Error *Error `json:"err"`
}

// Err is just a simple example error.
func (r ExampleResponse) Err() error {
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
