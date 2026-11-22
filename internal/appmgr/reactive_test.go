package appmgr

import (
	"reflect"
	"testing"

	"roundos/internal/wire"
)

func TestRetainedViewRejectsReplaysAndPartialInvalidPatches(t *testing.T) {
	v := wire.View{Screens: []wire.Screen{{ID: "root", Title: "Timer", Hero: "10", Widgets: []wire.Widget{{ID: "button", Kind: "button", Label: "Start", Action: "start"}}}}}
	full := wire.Message{Version: wire.Version, Type: "view", View: &v, LayoutRevision: 1, NewRevision: 1}
	var retained retainedView
	if _, ok := retained.accept(full); !ok {
		t.Fatal("initial full view rejected")
	}
	text := "09"
	valid := wire.PropertyUpdate{Screen: "root", Property: "hero", String: &text}
	patch := wire.Message{Version: wire.Version, Type: "widget_patch", LayoutRevision: 1, BaseRevision: 1, NewRevision: 2, Updates: []wire.PropertyUpdate{valid}}
	badBatch := patch
	badBatch.Updates = append(append([]wire.PropertyUpdate(nil), patch.Updates...), wire.PropertyUpdate{Screen: "root", Widget: "button", Property: "action", String: &text})
	wrongLayout := patch
	wrongLayout.LayoutRevision = 2
	wrongBase := patch
	wrongBase.BaseRevision = 2
	wrongNext := patch
	wrongNext.NewRevision = 3
	for _, bad := range []wire.Message{full, badBatch, wrongLayout, wrongBase, wrongNext} {
		before := wire.CopyView(*retained.view)
		if _, ok := retained.accept(bad); ok {
			t.Fatalf("invalid message accepted: %+v", bad)
		}
		if !reflect.DeepEqual(before, *retained.view) || retained.revision != 1 || retained.layout != 1 {
			t.Fatal("rejected message changed baseline")
		}
	}
	if next, ok := retained.accept(patch); !ok || next.Screens[0].Hero != "09" {
		t.Fatal(next, ok)
	}
	if _, ok := retained.accept(patch); ok {
		t.Fatal("duplicate patch accepted")
	}
	var reconnected retainedView
	if _, ok := reconnected.accept(patch); ok {
		t.Fatal("reconnected app patched without full snapshot")
	}
	full.LayoutRevision = 2
	if _, ok := retained.accept(full); !ok {
		t.Fatal("authoritative resync rejected")
	}
}
