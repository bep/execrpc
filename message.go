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

// Header is the header of a message.
// ID and Size are set by the system.
// Status may be set by the system.
type Header struct {
	ID      uint32
	Version uint16
	Status  uint16
	Size    uint32
}

const headerSize = 12

// Read reads the header from the reader.
func (h *Header) Read(r io.Reader) error {
	buf := make([]byte, headerSize)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return err
	}
	h.ID = binary.BigEndian.Uint32(buf[0:4])
	h.Version = binary.BigEndian.Uint16(buf[4:6])
	h.Status = binary.BigEndian.Uint16(buf[6:8])
	h.Size = binary.BigEndian.Uint32(buf[8:])
	return nil
}

// Write writes the header to the writer.
func (h Header) Write(w io.Writer) error {
	buff := make([]byte, headerSize)
	binary.BigEndian.PutUint32(buff[0:4], h.ID)
	binary.BigEndian.PutUint16(buff[4:6], h.Version)
	binary.BigEndian.PutUint16(buff[6:8], h.Status)
	binary.BigEndian.PutUint32(buff[8:], h.Size)
	_, err := w.Write(buff)
	return err
}
