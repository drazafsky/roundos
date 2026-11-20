package gesture

import "testing"

func TestThreeFingerCoherentSwipeEarly(t *testing.T) {
	d := New()
	d.Process(f(0, c(1, 95, 130), c(2, 190, 130), c(3, 280, 130)))
	ev := d.Process(f(110, c(1, 175, 132), c(2, 270, 131), c(3, 360, 130)))
	if !contains(ev, ThreeSwipe) {
		t.Fatal(ev)
	}
	if contains(d.Process(f(120, c(1, 190, 132), c(2, 285, 131), c(3, 375, 130))), ThreeSwipe) {
		t.Fatal("duplicate")
	}
	if contains(d.Process(f(160, c(1, 190, 132), c(3, 375, 130))), Tap) {
		t.Fatal("ghost")
	}
	if contains(d.Process(f(200)), Tap) {
		t.Fatal("ghost on final lift")
	}
}
func TestThreeFingerTransformIsNotSwipe(t *testing.T) {
	d := New()
	d.Process(f(0, c(1, 110, 190), c(2, 230, 190), c(3, 345, 190)))
	ev := d.Process(f(90, c(1, 55, 190), c(2, 230, 190), c(3, 400, 190)))
	if contains(ev, ThreeSwipe) {
		t.Fatal(ev)
	}
}
func TestFourFingerTapAndFifthContactSuppression(t *testing.T) {
	d := New()
	d.Process(f(0, c(1, 155, 150), c(2, 260, 150), c(3, 155, 270), c(4, 265, 270)))
	d.Process(f(110, c(1, 155, 150), c(2, 260, 150), c(3, 155, 270)))
	if !contains(d.Process(f(180)), FourTap) {
		t.Fatal("expected four tap")
	}
	d = New()
	d.Process(f(0, c(1, 110, 110), c(2, 220, 110), c(3, 325, 110), c(4, 170, 220), c(5, 275, 220)))
	d.Process(f(80, c(1, 110, 110), c(2, 220, 110), c(3, 325, 110), c(4, 170, 220)))
	if contains(d.Process(f(150)), FourTap) {
		t.Fatal("five fingers cannot become four tap")
	}
}
func TestFourTapMoveAndCancel(t *testing.T) {
	d := New()
	d.Process(f(0, c(1, 160, 130), c(2, 260, 130), c(3, 160, 250), c(4, 260, 250)))
	d.Process(f(100, c(1, 200, 130), c(2, 260, 130), c(3, 160, 250), c(4, 260, 250)))
	if contains(d.Process(f(180)), FourTap) {
		t.Fatal("moved finger")
	}
	d = New()
	d.Process(f(0, c(1, 160, 130), c(2, 260, 130), c(3, 160, 250), c(4, 260, 250)))
	d.Process(f(60, c(1, 160, 130), c(2, 260, 130), c(3, 160, 250)))
	d.Process(f(100))
	if contains(d.Process(f(150)), FourTap) {
		t.Fatal("duplicate")
	}
}

func TestTwoFingerSwipeCannotTurnIntoThreeFingerSwitch(t *testing.T) {
	d := New()
	d.Process(f(0, c(1, 90, 160), c(2, 200, 160)))
	if !contains(d.Process(f(100, c(1, 185, 160), c(2, 295, 160))), TwoSwipe) {
		t.Fatal("expected two swipe")
	}
	if contains(d.Process(f(140, c(1, 200, 160), c(2, 310, 160), c(3, 350, 250))), ThreeSwipe) {
		t.Fatal("phantom three swipe")
	}
	if contains(d.Process(f(190, c(1, 290, 160), c(2, 400, 160), c(3, 440, 250))), ThreeSwipe) {
		t.Fatal("late three swipe")
	}
	d.Process(f(230, c(1, 290, 160)))
	if contains(d.Process(f(270)), Tap) {
		t.Fatal("residual tap")
	}
}
func TestFiveContactEventAndNoFourTap(t *testing.T) {
	d := New()
	ev := d.Process(f(0, c(1, 115, 120), c(2, 215, 120), c(3, 320, 120), c(4, 170, 245), c(5, 280, 245)))
	if !contains(ev, FiveContacts) {
		t.Fatal("five contacts event missing")
	}
	ev = d.Process(f(120, c(1, 115, 120), c(2, 215, 120), c(3, 320, 120), c(4, 170, 245)))
	if contains(ev, FourTap) || contains(d.Process(f(150)), FourTap) {
		t.Fatal("false four-tap")
	}
}
