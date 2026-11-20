package wire

import (
	"bytes"
	"reflect"
	"testing"
)

func TestViewPatchRoundTripAndClears(t *testing.T) {
	old := View{Title: "Timer", Primary: "04:59", Hint: "running", ButtonAction: "toggle", RotaryAction: "adjust"}
	next := View{Title: "Timer", Primary: "04:58", Hint: "", ButtonAction: "toggle", RotaryAction: "adjust"}
	patch, n := DiffView(old, next)
	if n != 2 || patch.Hint == nil || *patch.Hint != "" {
		t.Fatalf("patch does not preserve field clearing: %+v (%d)", patch, n)
	}
	if got := patch.Apply(old); !reflect.DeepEqual(got, next) {
		t.Fatalf("roundtrip mismatch: %#v != %#v", got, next)
	}
	if _, n := DiffView(next, next); n != 0 {
		t.Fatalf("identical view generated %d changes", n)
	}
}

func TestViewPatchOnWire(t *testing.T) {
	changed := "03:42"
	original := View{Title: "Timer", Primary: "03:43", ButtonAction: "toggle"}
	msg := Message{Version: Version, Type: "view_patch", App: TimerID, Patch: &ViewPatch{Primary: &changed}}
	var buffer bytes.Buffer
	if err := Write(&buffer, msg); err != nil {
		t.Fatal(err)
	}
	got, err := Read(&buffer)
	if err != nil {
		t.Fatal(err)
	}
	if got.Patch == nil {
		t.Fatal("missing patch")
	}
	expected := original
	expected.Primary = changed
	if result := got.Patch.Apply(original); !reflect.DeepEqual(result, expected) {
		t.Fatalf("got %+v, want %+v", result, expected)
	}
}
