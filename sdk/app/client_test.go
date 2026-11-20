package app

import (
	"roundos/internal/wire"
	"testing"
)

func TestPublicGestureEvent(t *testing.T) {
	wireMessage := wire.Message{Type: "gesture", Gesture: &wire.Gesture{Kind: "pinch", X: 123, Y: 234, Scale: 1.4, Angle: 13, Fingers: 2}}
	ev := fromWire(wireMessage)
	if ev.Type != "gesture" || ev.Gesture == nil || ev.Gesture.Type != "pinch" || ev.Gesture.Fingers != 2 || ev.Gesture.Scale != 1.4 {
		t.Fatal(ev)
	}
}
