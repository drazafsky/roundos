package shell

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"testing"
	"time"

	"roundos/internal/appearance"
)

func TestAppIconsFollowLiveThemeAndAccessibility(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	iconBounds := image.Rect(211, 209, 270, 268)
	for selected := 0; selected < 3; selected++ {
		t.Run(fmt.Sprint(selected), func(t *testing.T) {
			m := New()
			m.Selected = selected
			var shape []byte
			for _, scale := range []int{100, 125, 150} {
				for _, large := range []bool{false, true} {
					for _, theme := range []string{appearance.Dark, appearance.Light, appearance.Contrast} {
						m.SetAppearance(appearance.Settings{Theme: theme, TextScale: scale, LargeTargets: large})
						if m.Selected != selected || m.Page != Home {
							t.Fatal("appearance changed launcher selection")
						}
						im := Draw(m, now)
						pal := m.Appearance.Palette()
						mask := iconMask(t, im, iconBounds, pal.Accent, pal.Selected)
						if shape == nil {
							shape = mask
						} else if !bytes.Equal(shape, mask) {
							t.Fatalf("icon changed/clipped for %s scale %d large targets %t", theme, scale, large)
						}
					}
				}
			}
			if changed, _ := m.Tap(240, 238); !changed || m.Page != []int{Clock, Timer, Settings}[selected] {
				t.Fatal("icon no longer activates its existing launcher target")
			}
		})
	}
}

func TestSwitcherIconsUseSelectedAndSecondaryThemeColors(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	for _, theme := range []string{appearance.Dark, appearance.Light, appearance.Contrast} {
		m := New()
		m.Appearance = appearance.Settings{Theme: theme, TextScale: 150, LargeTargets: true}
		m.OpenSwitcher()
		pal := m.Appearance.Palette()
		for _, focus := range []int{0, 1, 6} {
			m.SwitcherFocus = focus
			im := Draw(m, now)
			y := 155 + (focus-max(0, focus-2))*70 + 7
			iconMask(t, im, image.Rect(114, y-18, 151, y+19), pal.Accent, pal.Surface)
			if focus == 1 {
				iconMask(t, im, image.Rect(114, 144, 151, 181), pal.SecondaryText, pal.Surface)
			}
		}
	}
}

// Check the displayed icon region, including its surrounding background: no
// cached color or text may leak into the symbol when appearance changes.
func iconMask(t *testing.T, im *image.RGBA, bounds image.Rectangle, foreground, background color.RGBA) []byte {
	t.Helper()
	mask := make([]byte, 0, bounds.Dx()*bounds.Dy())
	count := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			switch pixel := im.RGBAAt(x, y); pixel {
			case foreground:
				mask = append(mask, 1)
				count++
			case background:
				mask = append(mask, 0)
			default:
				t.Fatalf("unexpected icon color at %d,%d: %v", x, y, pixel)
			}
		}
	}
	if count < 60 {
		t.Fatalf("missing or clipped icon: %d foreground pixels", count)
	}
	return mask
}
