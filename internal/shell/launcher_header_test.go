package shell

import (
	"fmt"
	"testing"
	"time"
)

func TestLauncherHeaderUsesDeviceLocalDateAndEnglishMonths(t *testing.T) {
	previous := time.Local
	t.Cleanup(func() { time.Local = previous })
	time.Local = time.FixedZone("DEVICE", -6*60*60)
	clock, date := launcherHeader(time.Date(2027, time.January, 1, 3, 4, 0, 0, time.UTC))
	if clock != "21:04" || date != "Dec. 31, 2026" {
		t.Fatalf("header did not convert the instant to device local time and date: %s / %s", clock, date)
	}
	months := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	for i, month := range months {
		_, date := launcherHeader(time.Date(2026, time.Month(i+1), 21, 12, 0, 0, 0, time.Local))
		if want := fmt.Sprintf("%s. 21, 2026", month); date != want {
			t.Fatalf("English abbreviated date: %q != %q", date, want)
		}
	}
}

func TestLauncherHeaderPositionsAndRemovedButton(t *testing.T) {
	m := New()
	now := time.Date(2026, time.November, 25, 12, 34, 0, 0, time.UTC)
	for _, scale := range []int{100, 125, 150} {
		m.Appearance.TextScale = scale
		actual := Draw(m, now)
		pal := m.Appearance.Palette()
		// A five-character time occupies the old branding band; the longer
		// date occupies the second band. Inspect the rendered foregrounds
		// without rebuilding the expected image through the same draw code.
		timeLeft, timeRight, dateLeft, dateRight := 480, 0, 480, 0
		for y := 79; y < 143; y++ {
			for x := 40; x < 440; x++ {
				if y < 110 && actual.RGBAAt(x, y) == pal.Accent {
					timeLeft, timeRight = min(timeLeft, x), max(timeRight, x)
				}
				if y >= 114 && actual.RGBAAt(x, y) == pal.PrimaryText {
					dateLeft, dateRight = min(dateLeft, x), max(dateRight, x)
				}
			}
		}
		if width := timeRight - timeLeft + 1; width < 60 || width > 116 {
			t.Fatalf("time missing or branding retained in first band, scale=%d width=%d", scale, width)
		}
		if width := dateRight - dateLeft + 1; width < 180 || width > 320 {
			t.Fatalf("date missing or clipped in second band, scale=%d width=%d", scale, width)
		}
		for y := 390; y < 432; y++ {
			for x := 154; x < 326; x++ {
				if actual.RGBAAt(x, y) != pal.Background {
					t.Fatal("removed Home button still renders")
				}
			}
		}
		m.Tap(240, 411)
		if m.Page != Home {
			t.Fatal("removed Home button still opens Diagnostics")
		}
		m.TouchUntil = time.Time{}
	}
}

func TestLauncherDateChangesAtLocalMidnight(t *testing.T) {
	previous := time.Local
	t.Cleanup(func() { time.Local = previous })
	time.Local = time.FixedZone("DEVICE", 9*60*60)
	before := time.Date(2026, time.November, 25, 14, 59, 59, 0, time.UTC)
	clock, date := launcherHeader(before)
	if clock != "23:59" || date != "Nov. 25, 2026" {
		t.Fatal(clock, date)
	}
	clock, date = launcherHeader(before.Add(time.Second))
	if clock != "00:00" || date != "Nov. 26, 2026" {
		t.Fatal(clock, date)
	}
}
