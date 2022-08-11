package model

type ExampleRequest struct {
	Text string `json:"text"`
}

type ExampleResponse struct {
	Hello string `json:"hello"`
	Error *Error `json:"err"`
}

func (r ExampleResponse) Err() error {
	if r.Error == nil {
		// Make sure that resp.Err() == nil.
		return nil
	}
	return r.Error
}

type Error struct {
	Msg string `json:"msg"`
}

func (r Error) Error() string {
	return r.Msg
}
