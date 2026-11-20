package wire

import (
	"bytes"
	"testing"
)

func TestNotificationAndContactFrameRoundTrip(t *testing.T) {
	for _, in := range []Message{
		{Version: Version, Type: "notify", Notification: &Notification{Title: "Timer finished", Body: "Completed", Priority: "actionable", Group: "timer", Action: "open"}},
		{Version: Version, Type: "subscribe_contacts", RawContacts: true},
		{Version: Version, Type: "contacts", Contacts: []Contact{{ID: 11, Slot: 0, X: 140, Y: 260}, {ID: 12, Slot: 1, X: 220, Y: 280}, {ID: 13, Slot: 2, X: 300, Y: 310}, {ID: 14, Slot: 3, X: 175, Y: 355}, {ID: 15, Slot: 4, X: 285, Y: 355}}},
	} {
		var buf bytes.Buffer
		if err := Write(&buf, in); err != nil {
			t.Fatal(err)
		}
		out, err := Read(&buf)
		if err != nil {
			t.Fatal(err)
		}
		if out.Type != in.Type || len(out.Contacts) != len(in.Contacts) {
			t.Fatalf("round trip %+v != %+v", out, in)
		}
	}
}
