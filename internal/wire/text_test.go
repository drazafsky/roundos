package wire

import (
	"bytes"
	"testing"
)

func TestTextProtocolRoundTrip(t *testing.T) {
	msg := Message{Version: Version, Type: "text_request", App: ClockID, TextRequest: &TextRequest{ID: "field-1", Label: "Label", Initial: "é世"}}
	var b bytes.Buffer
	if err := Write(&b, msg); err != nil {
		t.Fatal(err)
	}
	decoded, err := Read(&b)
	if err != nil || decoded.TextRequest == nil || decoded.TextRequest.Initial != "é世" {
		t.Fatalf("%+v %v", decoded, err)
	}
	result := Message{Version: Version, Type: "text_result", TextResult: &TextResult{ID: "field-1", Canceled: true}}
	b.Reset()
	if err := Write(&b, result); err != nil {
		t.Fatal(err)
	}
	decoded, err = Read(&b)
	if err != nil || decoded.TextResult == nil || !decoded.TextResult.Canceled || decoded.TextResult.Value != "" {
		t.Fatalf("%+v %v", decoded, err)
	}
}
