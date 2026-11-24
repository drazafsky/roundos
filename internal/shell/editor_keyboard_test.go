package shell

import (
	"image"
	"strings"
	"testing"
	"time"

	"roundos/internal/wire"
)

// Keep wheel highlighting outside the character grid so a changed selection
// cannot disguise a keyboard that still draws the previous group's keys.
func drawEditorKeyboard(m *Model) *image.RGBA {
	m.Editor.Wheel = len(m.Editor.WheelItems()) - 1 // DONE
	m.TouchUntil = time.Time{}                      // Exclude the transient tap marker from the comparison.
	return Draw(m, time.Unix(0, 0))
}

func sameEditorKeys(a, b *image.RGBA) bool {
	for y := 198; y < 347; y++ {
		for x := 46; x < 436; x++ {
			if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
				return false
			}
		}
	}
	return true
}

func TestTouchKeyboardSwitchesDisplayedAndInsertedCharacters(t *testing.T) {
	for _, secure := range []bool{false, true} {
		name := "public"
		if secure {
			name = "secure"
		}
		t.Run(name, func(t *testing.T) {
			m := New()
			m.Select(Clock)
			m.OpenAppEditor(wire.ClockID, wire.TextRequest{ID: "label", Secure: secure})
			letters := drawEditorKeyboard(m)
			m.Tap(240, 180) // 123 tab
			if m.Editor.Group != 1 || m.Editor.Draft != "" || m.Editor.Cursor != 0 {
				t.Fatal("changing the group changed the draft or cursor")
			}
			numbers := drawEditorKeyboard(m)
			if sameEditorKeys(letters, numbers) {
				t.Error("123 tab left the displayed character keys unchanged")
			}
			m.Tap(61, 218)  // 0
			m.Tap(421, 218) // 9
			if m.Editor.Draft != "09" {
				t.Fatalf("numeric touch keys inserted %q, want 09", m.Editor.Draft)
			}
			if changed, _ := m.Tap(85, 270); changed || m.Editor.Draft != "09" {
				t.Fatal("empty numeric row accepted a stale alphabetic key")
			}
			m.Tap(340, 180) // SYM tab
			if m.Editor.Cursor != 2 || m.Editor.Draft != "09" {
				t.Fatal("changing to symbols lost the draft or cursor")
			}
			if sameEditorKeys(numbers, drawEditorKeyboard(m)) {
				t.Error("SYM tab left the displayed character keys unchanged")
			}
			m.Tap(61, 218)  // .
			m.Tap(421, 218) // &
			m.Tap(103, 270) // +
			m.Tap(376, 270) // _
			if m.Editor.Draft != "09.&+_" {
				t.Fatalf("symbol touch keys inserted %q", m.Editor.Draft)
			}
			if changed, _ := m.Tap(120, 320); changed || m.Editor.Draft != "09.&+_" {
				t.Fatal("empty symbol row accepted a stale alphabetic key")
			}
			m.Tap(140, 180) // ABC tab
			if !sameEditorKeys(letters, drawEditorKeyboard(m)) {
				t.Error("ABC did not restore the original keyboard")
			}
			m.Tap(61, 218)
			m.Tap(125, 364) // SHIFT
			m.Tap(61, 218)
			want := "09.&+_qQ"
			if m.Editor.Draft != want {
				t.Fatalf("alphabetic case after group changes: %q", m.Editor.Draft)
			}
			if secure && m.Editor.Masked() != strings.Repeat("*", len(want)) {
				t.Fatal("secure draft was not masked")
			}
			m.Tap(240, 413) // DONE
			app, result := m.TakeTextResult()
			if m.Page != Clock || app != wire.ClockID || result == nil || result.Canceled || result.Value != want {
				t.Fatal("mixed character input did not reach the requesting app")
			}
			if secure && (m.Editor.Draft != "" || m.Editor.Committed != "") {
				t.Fatal("secure contents remained after editor completion")
			}
		})
	}
}

func TestTouchKeyboardExposesEveryNumberAndSymbol(t *testing.T) {
	m := New()
	m.OpenEditor()
	for _, row := range []struct {
		tab, x, y int
		keys      string
	}{
		{240, 46, 218, "0123456789"},
		{340, 46, 218, ".,!?@#-$%&"},
		{340, 85, 270, "+/=:;()_"},
	} {
		m.Tap(row.tab, 180)
		for i := range row.keys {
			m.Tap(row.x+18+i*39, row.y)
		}
	}
	if m.Editor.Draft != "0123456789.,!?@#-$%&+/=:;()_" {
		t.Fatalf("unreachable or incorrect touchscreen characters: %q", m.Editor.Draft)
	}
}
