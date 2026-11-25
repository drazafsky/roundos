package shell

import (
	"bytes"
	"image"
	"math"
	"testing"
	"time"

	"roundos/internal/appmgr"
)

func TestCarouselWrapPreferenceAndInputBoundaries(t *testing.T) {
	m := New()
	if !m.Appearance.CarouselWrap {
		t.Fatal("wrapping must default to enabled")
	}
	m.OpenAccessibility()
	m.Rotate(4)
	if changed, _ := m.Activate(); !changed || m.Appearance.CarouselWrap || !m.AppearanceDirty {
		t.Fatal("rotary cannot disable and persist wrapping")
	}
	m.Back()
	for _, delta := range []int{-1, -10} {
		if changed, _, _ := m.Rotate(delta); changed || m.Selected != 0 {
			t.Fatal("disabled wrapping crossed the first app")
		}
	}
	m.Rotate(100)
	if m.Selected != m.CarouselCount()-1 {
		t.Fatal("nonwrapping rotary must clamp to last app")
	}
	if changed, _ := m.Tap(417, 238); changed || m.Selected != 2 {
		t.Fatal("empty right neighbor must not wrap")
	}
	if changed, _ := m.Tap(63, 238); !changed || m.Selected != 1 {
		t.Fatal("left neighbor must remain reachable")
	}
	m.OpenAccessibility()
	m.AccessFocus = 4
	_, _, top, _, _ := accessGeometry(m)
	// Focus four scrolls the list to rows two through four.
	if changed, _ := m.Tap(240, top+2*76+20); !changed || !m.Appearance.CarouselWrap {
		t.Fatal("touch cannot re-enable wrapping")
	}
	m.Back()
	m.Selected = 0
	if changed, _ := m.Tap(63, 238); !changed || m.Selected != 2 {
		t.Fatal("touch wrapping did not return after enabling")
	}
}

func TestCarouselAnimationRetargetWrapAndStop(t *testing.T) {
	m := New()
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	m.moveCarousel(-1, now)
	if m.Selected != 2 || m.carouselPosition(now) != 0 {
		t.Fatal("wrap must change selection while keeping the initial visual position")
	}
	mid := now.Add(carouselDuration / 2)
	if got := m.carouselPosition(mid); math.Abs(got+0.5) > 0.001 {
		t.Fatalf("wrap took a long path instead of sliding one app: %v", got)
	}
	before := Draw(m, now)
	middle := Draw(m, mid)
	if bytes.Equal(before.Pix, middle.Pix) {
		t.Fatal("focus changed without an intermediate slide")
	}
	m.moveCarousel(1, mid)
	if got := m.carouselPosition(mid); math.Abs(got+0.5) > 0.001 {
		t.Fatal("new input jumped away from the current visual position")
	}
	snapshot := m.Snapshot()
	if !bytes.Equal(Draw(snapshot, mid).Pix, Draw(m, mid).Pix) {
		t.Fatal("snapshot lost the interrupted slide")
	}
	if next := m.NextCarouselWake(mid); next <= 0 || next > 20*time.Millisecond {
		t.Fatalf("animation did not schedule a frame: %v", next)
	}
	end := mid.Add(carouselDuration)
	if !m.AdvanceCarousel(end) || m.NextCarouselWake(end) != 0 || m.AdvanceCarousel(end) {
		t.Fatal("animation failed to paint its final frame and stop waking")
	}
	if m.Selected != 0 || m.carouselPosition(end) != 0 {
		t.Fatal("interrupted slide finished on the wrong app")
	}
}

func TestCarouselReducedMotionAndNavigationCancelAnimation(t *testing.T) {
	m := New()
	now := time.Now()
	m.moveCarousel(1, now)
	s := m.Appearance
	s.ReduceMotion = true
	m.SetAppearance(s)
	if m.carouselPosition(now) != 1 || m.NextCarouselWake(now) != 0 {
		t.Fatal("live reduced motion did not settle the current transition")
	}
	m.moveCarousel(1, now)
	if m.Selected != 2 || m.carouselPosition(now) != 2 || m.NextCarouselWake(now) != 0 {
		t.Fatal("reduced motion must change focus immediately without frame wakes")
	}
	s.ReduceMotion = false
	m.SetAppearance(s)
	m.moveCarousel(1, now)
	m.Select(Clock)
	m.Select(Home)
	if m.NextCarouselWake(now) != 0 || m.carouselPosition(now) != 0 {
		t.Fatal("leaving and returning replayed a stale transition")
	}
	m.moveCarousel(1, now)
	m.SetInstalledApps(nil)
	if m.NextCarouselWake(now) != 0 {
		t.Fatal("registry change retained a stale animation")
	}
}

func TestCarouselNamesUnderEveryInstalledIcon(t *testing.T) {
	m := New()
	m.SetInstalledApps([]appmgr.AppSpec{{ID: "org.example.notes", Name: "Notes", Icon: "NTE"}})
	now := time.Now()
	for focus := range m.AppCards {
		m.Selected = focus
		withNames := Draw(m, now)
		snapshot := m.Snapshot()
		for i := range snapshot.AppCards {
			snapshot.AppCards[i].Title = ""
		}
		withoutNames := Draw(snapshot, now)
		for _, x := range []int{63, 240, 417} {
			region := image.Rect(x-25, 294, x+25, 334)
			if bytes.Equal(imageBytes(withNames, region), imageBytes(withoutNames, region)) {
				t.Fatalf("missing app name underneath an icon: focus=%d x=%d", focus, x)
			}
		}
	}
}

func imageBytes(im *image.RGBA, region image.Rectangle) []byte {
	var out []byte
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			pixel := im.RGBAAt(x, y)
			out = append(out, pixel.R, pixel.G, pixel.B, pixel.A)
		}
	}
	return out
}
