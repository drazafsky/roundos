package touch

import (
	"testing"
	"time"
)

func decoder(t *testing.T) *Decoder {
	t.Helper()
	d, err := newDecoder(axisInfo{Min: 0, Max: 479}, axisInfo{Min: 0, Max: 479}, true)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func feed(d *Decoder, typ, code uint16, v int) (Frame, bool) {
	return d.Feed(typ, code, v, time.Unix(0, 0))
}
func TestTypeBTwoContactsAndSlotTracking(t *testing.T) {
	d := decoder(t)
	feed(d, evAbs, absMTSlot, 0)
	feed(d, evAbs, absMTTrackingID, 41)
	feed(d, evAbs, absMTPositionX, 110)
	feed(d, evAbs, absMTPositionY, 120)
	feed(d, evAbs, absMTSlot, 1)
	feed(d, evAbs, absMTTrackingID, 99)
	feed(d, evAbs, absMTPositionX, 300)
	feed(d, evAbs, absMTPositionY, 320)
	f, ok := feed(d, evSyn, synReport, 0)
	if !ok || len(f.Contacts) != 2 || f.Contacts[0].ID != 41 || f.Contacts[1].ID != 99 {
		t.Fatalf("two contacts: %+v %v", f, ok)
	}
	// type B reports only updated slots in a subsequent frame.
	feed(d, evAbs, absMTSlot, 1)
	feed(d, evAbs, absMTPositionX, 310)
	f, ok = feed(d, evSyn, synReport, 0)
	if !ok || f.Contacts[0].X != 110 || f.Contacts[1].X != 310 {
		t.Fatalf("retained slot coordinates: %+v", f)
	}
	if _, ok := feed(d, evSyn, synReport, 0); ok {
		t.Fatal("unchanged frame should not wake shell")
	}
	feed(d, evAbs, absMTTrackingID, -1)
	f, ok = feed(d, evSyn, synReport, 0)
	if !ok || len(f.Contacts) != 1 || f.Contacts[0].ID != 41 {
		t.Fatalf("one finger release: %+v", f)
	}
	feed(d, evAbs, absMTSlot, 0)
	feed(d, evAbs, absMTTrackingID, -1)
	f, ok = feed(d, evSyn, synReport, 0)
	if !ok || len(f.Contacts) != 0 {
		t.Fatalf("all fingers released: %+v", f)
	}
}
func TestNoPartialContactsOrStaleTrackingIDs(t *testing.T) {
	d := decoder(t)
	feed(d, evAbs, absMTSlot, 2)
	feed(d, evAbs, absMTTrackingID, 10)
	if _, ok := feed(d, evSyn, synReport, 0); ok {
		t.Fatal("missing X/Y should not emit touch")
	}
	feed(d, evAbs, absMTPositionX, 50)
	feed(d, evAbs, absMTPositionY, 60)
	f, ok := feed(d, evSyn, synReport, 0)
	if !ok || f.Contacts[0].Slot != 2 {
		t.Fatal(f)
	}
	feed(d, evAbs, absMTTrackingID, -1)
	if f, ok = feed(d, evSyn, synReport, 0); !ok || len(f.Contacts) != 0 {
		t.Fatal(f)
	}
	feed(d, evAbs, absMTTrackingID, 11)
	if f, ok = feed(d, evSyn, synReport, 0); ok {
		t.Fatalf("reused slot must not inherit old coords: %+v", f)
	}
}
func TestSYN_DROPPEDCancelsWithoutGhostTap(t *testing.T) {
	d := decoder(t)
	feed(d, evAbs, absMTTrackingID, 7)
	feed(d, evAbs, absMTPositionX, 150)
	feed(d, evAbs, absMTPositionY, 150)
	feed(d, evSyn, synReport, 0)
	f, ok := feed(d, evSyn, synDropped, 0)
	if !ok || !f.Canceled || len(f.Contacts) != 0 {
		t.Fatalf("cancel: %+v", f)
	}
	feed(d, evAbs, absMTPositionX, 200)
	if _, ok = feed(d, evSyn, synReport, 0); ok {
		t.Fatal("dropped sync produced frame")
	}
	if _, ok = feed(d, evSyn, synReport, 0); ok {
		t.Fatal("ghost contact after dropped sync")
	}
}
func TestSingleTouchFallback(t *testing.T) {
	d, err := newDecoder(axisInfo{Min: 100, Max: 1100}, axisInfo{Min: 0, Max: 1000}, false)
	if err != nil {
		t.Fatal(err)
	}
	feed(d, evAbs, absX, 600)
	feed(d, evAbs, absY, 500)
	feed(d, evKey, btnTouch, 1)
	f, ok := feed(d, evSyn, synReport, 0)
	if !ok || len(f.Contacts) != 1 || f.Contacts[0].X != 240 || f.Contacts[0].Y != 240 {
		t.Fatal(f)
	}
	feed(d, evKey, btnTouch, 0)
	f, ok = feed(d, evSyn, synReport, 0)
	if !ok || len(f.Contacts) != 0 {
		t.Fatal(f)
	}
}
