package execrpc

import (
	"encoding/binary"
	"io"
)

// Message is what gets sent to and from the server.
type Message struct {
	Header Header
	Body   []byte
}

func (m *Message) Read(r io.Reader) error {
	if err := m.Header.Read(r); err != nil {
		return err
	}
	m.Body = make([]byte, m.Header.Size)
	_, err := io.ReadFull(r, m.Body)
	return err
}

func (m *Message) Write(w io.Writer) error {
	m.Header.Size = uint32(len(m.Body))
	if err := m.Header.Write(w); err != nil {
		return err
	}
	_, err := w.Write(m.Body)
	return err
}

type Header struct {
	ID      uint32
	Version uint8
	Status  uint8
	Size    uint32
}

func (h *Header) Read(r io.Reader) error {
	buf := make([]byte, 10)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return err
	}
	h.ID = binary.BigEndian.Uint32(buf[0:4])
	h.Version = buf[4]
	h.Status = buf[5]
	h.Size = binary.BigEndian.Uint32(buf[6:])
	return nil
}

func (h Header) Write(w io.Writer) error {
	buff := make([]byte, 10)
	binary.BigEndian.PutUint32(buff[0:4], h.ID)
	buff[4] = h.Version
	buff[5] = h.Status
	binary.BigEndian.PutUint32(buff[6:], h.Size)
	_, err := w.Write(buff)
	return err
}
