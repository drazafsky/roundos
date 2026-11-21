// Package devproto is the bounded compiled-bundle development control protocol.
// It is separate from authenticated application IPC and carries no app state.
package devproto

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

const MaxFrame = 1 << 20

type Request struct {
	Token  string `json:"token,omitempty"`
	Bundle []byte `json:"bundle"`
}
type Response struct {
	Error string `json:"error,omitempty"`
	Retry bool   `json:"retry,omitempty"`
}

func Read(reader io.Reader, value any) error {
	var header [4]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(header[:])
	if n == 0 || n > MaxFrame {
		return fmt.Errorf("invalid development frame size")
	}
	data := make([]byte, int(n))
	if _, err := io.ReadFull(reader, data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("trailing development frame data")
	}
	return nil
}

func Write(writer io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data) == 0 || len(data) > MaxFrame {
		return fmt.Errorf("development frame exceeds budget")
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(data)))
	for _, part := range [][]byte{header[:], data} {
		for len(part) > 0 {
			n, err := writer.Write(part)
			if err != nil {
				return err
			}
			if n == 0 {
				return io.ErrShortWrite
			}
			part = part[n:]
		}
	}
	return nil
}
