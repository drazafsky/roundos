// Package circleui owns geometry and focus navigation for a 480x480 circular
// display. No input handler should maintain independent touch rectangles.
package circleui

import (
	"math"
	"roundos/internal/wire"
)

const Center = 240
const SafeRadius = 218

type Rect struct{ Left, Top, Right, Bottom int }

func (r Rect) Hit(x, y int) bool { return x >= r.Left && x < r.Right && y >= r.Top && y < r.Bottom }

type Row struct {
	Index int
	Rect  Rect
}

func Focusable(w wire.Widget) bool {
	return !w.Disabled && (w.Kind == "button" || w.Kind == "toggle" || w.Kind == "slider" || w.Kind == "picker" || w.Kind == "time_picker" || w.Kind == "date_picker" || w.Kind == "list_item")
}
func First(items []wire.Widget) int {
	for i, w := range items {
		if Focusable(w) {
			return i
		}
	}
	return -1
}
func Next(items []wire.Widget, focus, delta int) int {
	if len(items) == 0 || delta == 0 {
		return focus
	}
	step := 1
	if delta < 0 {
		step = -1
	}
	for k := 0; k < abs(delta); k++ {
		candidate := focus + step
		for candidate >= 0 && candidate < len(items) && !Focusable(items[candidate]) {
			candidate += step
		}
		if candidate < 0 || candidate >= len(items) {
			break
		}
		focus = candidate
	}
	return focus
}
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// BackRect is the consistent touch and rendering region on nested screens.
func BackRect() Rect { return Rect{Left: 89, Top: 82, Right: 156, Bottom: 147} }

func layout(items []wire.Widget, focus, scroll int, hero, largeTargets bool) []Row {
	count, top, step, height := 3, 159, 78, 67
	if hero {
		count, top, step, height = 2, 281, 79, 62
	}
	if largeTargets && !hero {
		count, top, step, height = 2, 167, 100, 86
	}
	if count > len(items) {
		count = len(items)
	}
	if count <= 0 {
		return nil
	}
	maxStart := len(items) - count
	if scroll < 0 {
		scroll = 0
	}
	if scroll > maxStart {
		scroll = maxStart
	}
	rows := make([]Row, 0, count)
	for i := 0; i < count; i++ {
		y := top + i*step
		far := math.Max(math.Abs(float64(y-Center)), math.Abs(float64(y+height-Center)))
		half := int(math.Sqrt(math.Max(0, SafeRadius*SafeRadius-far*far))) - 9
		if half > 148 {
			half = 148
		}
		if half < 35 {
			continue
		}
		rows = append(rows, Row{Index: scroll + i, Rect: Rect{Left: Center - half, Top: y, Right: Center + half, Bottom: y + height}})
	}
	return rows
}

// ScrollTo keeps the selected row visible with no wraparound.
func ScrollTo(items []wire.Widget, focus, scroll int, hero, largeTargets bool) int {
	count := 3
	if hero || largeTargets {
		count = 2
	}
	return scrollToCount(items, focus, scroll, count)
}

func scrollToCount(items []wire.Widget, focus, scroll, count int) int {
	if focus < scroll {
		scroll = focus
	}
	if focus >= scroll+count {
		scroll = focus - count + 1
	}
	return max(0, min(scroll, max(0, len(items)-count)))
}
func VisibleAt(items []wire.Widget, focus, scroll int, hero, largeTargets bool) []Row {
	scroll = ScrollTo(items, focus, scroll, hero, largeTargets)
	return layout(items, focus, scroll, hero, largeTargets)
}
func Visible(items []wire.Widget, focus int, hero, largeTargets bool) []Row {
	return VisibleAt(items, focus, 0, hero, largeTargets)
}
func Hit(rows []Row, x, y int) int {
	for _, row := range rows {
		if row.Rect.Hit(x, y) {
			return row.Index
		}
	}
	return -1
}

// VisiblePixelAt is the same circle-aware layout with continuous, pixel-level
// scrolling. Both renderer and hit tester consume the identical clipped rows.
// Pixel offsets are bounded; partially visible rows never overlap the header
// or escape the 218-pixel safe radius.
func RowMetrics(hero, largeTargets bool) (count, top, step, height int) {
	count, top, step, height = 3, 159, 78, 67
	if hero {
		count, top, step, height = 2, 281, 79, 62
	}
	if largeTargets && !hero {
		count, top, step, height = 2, 167, 100, 86
	}
	return
}
func MaxScrollPx(n int, hero, largeTargets bool) float64 {
	count, _, step, _ := RowMetrics(hero, largeTargets)
	return float64(max(0, n-count) * step)
}
func VisiblePixelAt(items []wire.Widget, scrollPx float64, hero, largeTargets bool) []Row {
	count, top, step, height := RowMetrics(hero, largeTargets)
	return visiblePixelMetrics(items, scrollPx, count, top, step, height)
}

// ScreenMetrics is shared by production drawing, preview, hit-testing,
// rotary focus and kinetic scrolling. Legacy and list screens retain their
// established geometry; forms and detail pages reserve taller control rows.
func ScreenMetrics(screen wire.Screen, largeTargets bool) (count, top, step, height int) {
	count, top, step, height = RowMetrics(screen.Hero != "", largeTargets)
	if screen.Hero != "" {
		return
	}
	switch screen.Layout {
	case "form":
		count, top, step, height = 2, 159, 98, 86
	case "detail":
		count, top, step, height = 2, 159, 110, 98
	}
	return
}

func ScreenMaxScrollPx(screen wire.Screen, largeTargets bool) float64 {
	count, _, step, _ := ScreenMetrics(screen, largeTargets)
	return float64(max(0, len(screen.Widgets)-count) * step)
}

func ScreenScrollTo(screen wire.Screen, focus, scroll int, largeTargets bool) int {
	count, _, _, _ := ScreenMetrics(screen, largeTargets)
	return scrollToCount(screen.Widgets, focus, scroll, count)
}

func VisibleScreenPixelAt(screen wire.Screen, scrollPx float64, largeTargets bool) []Row {
	count, top, step, height := ScreenMetrics(screen, largeTargets)
	return visiblePixelMetrics(screen.Widgets, scrollPx, count, top, step, height)
}

func visiblePixelMetrics(items []wire.Widget, scrollPx float64, count, top, step, height int) []Row {
	if len(items) == 0 {
		return nil
	}
	scrollPx = math.Max(0, math.Min(float64(max(0, len(items)-count)*step), scrollPx))
	viewportEnd := top + (count-1)*step + height
	rows := make([]Row, 0, count+2)
	first := max(0, int(math.Floor(scrollPx/float64(step))))
	for i := first; i < len(items); i++ {
		y := top + i*step - int(math.Round(scrollPx))
		if y >= viewportEnd {
			break
		}
		rowTop, rowBottom := max(top, y), min(viewportEnd, y+height)
		if rowBottom-rowTop < 15 {
			continue
		}
		far := math.Max(math.Abs(float64(rowTop-Center)), math.Abs(float64(rowBottom-Center)))
		half := int(math.Sqrt(math.Max(0, SafeRadius*SafeRadius-far*far))) - 9
		if half > 148 {
			half = 148
		}
		if half < 35 {
			continue
		}
		rows = append(rows, Row{Index: i, Rect: Rect{Left: Center - half, Top: rowTop, Right: Center + half, Bottom: rowBottom}})
	}
	return rows
}
