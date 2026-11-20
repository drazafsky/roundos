package wire

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	orig := Message{Version: Version, Type: "view", App: TimerID, View: &View{Title: "Timer", Primary: "04:50", ButtonAction: "toggle"}}
	var b bytes.Buffer
	if err := Write(&b, orig); err != nil {
		t.Fatal(err)
	}
	got, err := Read(&b)
	if err != nil {
		t.Fatal(err)
	}
	if got.View == nil || got.View.Primary != "04:50" || got.App != TimerID {
		t.Fatalf("unexpected roundtrip: %+v", got)
	}
}
func TestRejectBadFrame(t *testing.T) {
	var b bytes.Buffer
	binary.Write(&b, binary.BigEndian, uint32(MaxMessage+1))
	if _, err := Read(&b); err == nil {
		t.Fatal("oversized frame accepted")
	}
}
func TestRejectProtocol(t *testing.T) {
	if err := Write(&bytes.Buffer{}, Message{Version: 1}); err == nil {
		t.Fatal("old version accepted")
	}
}
func TestRejectHugeMessage(t *testing.T) {
	err := Write(&bytes.Buffer{}, Message{Version: Version, Type: strings.Repeat("x", MaxMessage)})
	if err == nil {
		t.Fatal("huge message accepted")
	}
}

func TestGestureRoundTrip(t *testing.T) {
	orig := Message{Version: Version, Type: "gesture", Gesture: &Gesture{Kind: "two_swipe", Fingers: 2, DX: 80, Direction: "right"}}
	var b bytes.Buffer
	if err := Write(&b, orig); err != nil {
		t.Fatal(err)
	}
	result, err := Read(&b)
	if err != nil || result.Gesture == nil || result.Gesture.Direction != "right" || result.Gesture.Fingers != 2 {
		t.Fatalf("gesture serialization: %+v %v", result, err)
	}
}
