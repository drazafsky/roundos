package devproto

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

type partialWriter struct{ bytes.Buffer }

func (w *partialWriter) Write(data []byte) (int, error) {
	if len(data) > 3 {
		data = data[:3]
	}
	return w.Buffer.Write(data)
}
func TestFramingBoundsAndStrictDecode(t *testing.T) {
	var wire partialWriter
	if err := Write(&wire, Request{Bundle: []byte("compiled")}); err != nil {
		t.Fatal(err)
	}
	var request Request
	if err := Read(&wire, &request); err != nil || string(request.Bundle) != "compiled" {
		t.Fatal("short-write round trip", err)
	}
	for _, body := range []string{`{"unknown":true}`, `{} {}`} {
		var packet bytes.Buffer
		binary.Write(&packet, binary.BigEndian, uint32(len(body)))
		packet.WriteString(body)
		if err := Read(&packet, &Request{}); err == nil {
			t.Fatal("invalid frame accepted", body)
		}
	}
	var oversized bytes.Buffer
	binary.Write(&oversized, binary.BigEndian, uint32(MaxFrame+1))
	if err := Read(&oversized, &request); err == nil {
		t.Fatal("oversized frame accepted")
	}
	if err := Read(bytes.NewReader(nil), &request); err != io.EOF {
		t.Fatal("clean EOF lost", err)
	}
}
