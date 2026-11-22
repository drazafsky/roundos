package shell

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"roundos/internal/circleui"
	"roundos/internal/wire"
	"roundos/sdk/ui"
)

func TestTemplateDrawingAndHitTestingShareGeometry(t *testing.T) {
	makeModel := func(layout string) *Model {
		m := New()
		screen := ui.Screen("root", "Test", "", "", ui.PrimaryButton("a", "Action", "act"), ui.Button("b", "Second", "other"))
		screen.Layout = layout
		m.SetView(wire.ClockID, ui.View{Screens: []ui.ScreenSpec{screen}})
		m.Select(Clock)
		return m
	}
	list, form := makeModel("list"), makeModel("form")
	if _, action := list.WidgetTap(240, 235); action != "" {
		t.Fatal("list gap activated a row")
	}
	if _, action := form.WidgetTap(240, 235); action != "act" {
		t.Fatal("form did not hit its taller first control")
	}
	if Draw(list, time.Unix(0, 0)).RGBAAt(240, 235) == Draw(form, time.Unix(0, 0)).RGBAAt(240, 235) {
		t.Fatal("drawing ignored the same geometry used by input")
	}
	for _, layout := range []string{"hero_list", "list", "form", "detail"} {
		m := makeModel(layout)
		screen, _ := m.CurrentScreen(Clock)
		if layout == "hero_list" {
			screen.Hero = "10"
		}
		for _, row := range circleui.VisibleScreenPixelAt(screen, 0, false) {
			for _, x := range []int{row.Rect.Left, row.Rect.Right - 1} {
				for _, y := range []int{row.Rect.Top, row.Rect.Bottom - 1} {
					dx, dy := x-240, y-240
					if dx*dx+dy*dy > 218*218 {
						t.Fatalf("%s row escapes circle: %+v", layout, row)
					}
				}
			}
		}
	}
}

func TestDetailWrappingBoundsAndTruncatesText(t *testing.T) {
	for _, text := range []string{strings.Repeat("Long detail text ", 20), strings.Repeat("界", 80), strings.Repeat("x", 100)} {
		lines := wrapDetail(text, 20, 3)
		if len(lines) != 3 || !strings.HasSuffix(lines[2], "...") {
			t.Fatal("missing bounded truncation", lines)
		}
		for _, line := range lines {
			if !utf8.ValidString(line) || len([]rune(line)) > 20 {
				t.Fatal("invalid or overflowing wrapped line", line)
			}
		}
	}
}
