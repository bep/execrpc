package execrpc

import (
	"bytes"
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestMessage(t *testing.T) {
	c := qt.New(t)

	m1 := Message{
		Body: []byte("hello"),
		Header: Header{
			ID:      2,
			Version: 3,
			Status:  4,
			Size:    5,
		},
	}

	var b bytes.Buffer

	c.Assert(m1.Header.Write(&b), qt.IsNil)
	c.Assert(m1.Write(&b), qt.IsNil)

	var m2 Message
	c.Assert(m2.Header.Read(&b), qt.IsNil)
	c.Assert(m2.Read(&b), qt.IsNil)

	c.Assert(m2, qt.DeepEquals, m1)

}
