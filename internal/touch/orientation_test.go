package touch

import "testing"

// Optional Y inversion is supported for other panels, but is NOT the default
// for the tested HyperPixel; 5.1.3's default inversion broke tap locations.
func TestExplicitYInversionForMultitouch(t *testing.T) {
	d := decoder(t)
	d.invertY = true
	feed(d, evAbs, absMTSlot, 0)
	feed(d, evAbs, absMTTrackingID, 9)
	feed(d, evAbs, absMTPositionX, 100)
	feed(d, evAbs, absMTPositionY, 430)
	feed(d, evAbs, absMTSlot, 1)
	feed(d, evAbs, absMTTrackingID, 10)
	feed(d, evAbs, absMTPositionX, 300)
	feed(d, evAbs, absMTPositionY, 400)
	f, ok := feed(d, evSyn, synReport, 0)
	if !ok || len(f.Contacts) != 2 || f.Contacts[0].Y != 49 || f.Contacts[1].Y != 79 {
		t.Fatalf("initial inverted contacts: %+v, ok=%v", f, ok)
	}
	// For panels requiring inverted coordinates, decreasing raw Y should
	// increase rendered Y. X and tracking IDs remain unchanged.
	feed(d, evAbs, absMTSlot, 0)
	feed(d, evAbs, absMTPositionY, 370)
	feed(d, evAbs, absMTSlot, 1)
	feed(d, evAbs, absMTPositionY, 340)
	f, ok = feed(d, evSyn, synReport, 0)
	if !ok || len(f.Contacts) != 2 || f.Contacts[0].Y != 109 || f.Contacts[1].Y != 139 || f.Contacts[0].X != 100 || f.Contacts[1].X != 300 || f.Contacts[0].ID != 9 {
		t.Fatalf("physical downward movement should increase screen-space Y: %+v", f)
	}
}

func TestYOrientationCanBeDisabled(t *testing.T) {
	d := decoder(t)
	d.invertY = false
	feed(d, evAbs, absMTTrackingID, 9)
	feed(d, evAbs, absMTPositionX, 100)
	feed(d, evAbs, absMTPositionY, 430)
	f, ok := feed(d, evSyn, synReport, 0)
	if !ok || len(f.Contacts) != 1 || f.Contacts[0].Y != 430 {
		t.Fatalf("normal coordinates must not be inverted: %+v", f)
	}
}

func TestExplicitYInversionForSingleTouchFallback(t *testing.T) {
	d, err := newDecoder(axisInfo{Min: 100, Max: 1100}, axisInfo{Min: 0, Max: 1000}, false)
	if err != nil {
		t.Fatal(err)
	}
	d.invertY = true
	feed(d, evAbs, absX, 600)
	feed(d, evAbs, absY, 1000)
	feed(d, evKey, btnTouch, 1)
	f, ok := feed(d, evSyn, synReport, 0)
	if !ok || len(f.Contacts) != 1 || f.Contacts[0].X != 240 || f.Contacts[0].Y != 0 {
		t.Fatalf("inverted single-touch position: %+v", f)
	}
	feed(d, evAbs, absY, 0)
	f, ok = feed(d, evSyn, synReport, 0)
	if !ok || f.Contacts[0].Y != 479 {
		t.Fatalf("inverted single-touch boundary: %+v", f)
	}
}

// Regression: default decoding must preserve raw touch location for the
// physically tested HyperPixel. Global inversion put lower controls at top.
func TestDefaultTouchPositionRemainsUnmirrored(t *testing.T) {
	d := decoder(t)
	if d.invertY {
		t.Fatal("default decoder must not invert Y")
	}
	feed(d, evAbs, absMTSlot, 0)
	feed(d, evAbs, absMTTrackingID, 7)
	feed(d, evAbs, absMTPositionX, 240)
	feed(d, evAbs, absMTPositionY, 390)
	f, ok := feed(d, evSyn, synReport, 0)
	if !ok || len(f.Contacts) != 1 || f.Contacts[0].Y != 390 {
		t.Fatalf("bottom control position must remain bottom: %+v", f)
	}
	feed(d, evAbs, absMTPositionY, 20)
	f, ok = feed(d, evSyn, synReport, 0)
	if !ok || len(f.Contacts) != 1 || f.Contacts[0].Y != 20 {
		t.Fatalf("top edge position must remain top: %+v", f)
	}
}
