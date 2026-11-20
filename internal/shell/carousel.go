package shell

import (
	"image"
	"time"
)

// Home carousel targets are Clock, Timer and Settings. The selected card is
// always the only launch target; neighboring cards change focus.
func drawCarousel(im *image.RGBA, m *Model, now time.Time) {
	p := m.Appearance.Palette()
	textTheme(im, 240, 79, 3, "ROUNDOS", p.Accent, true, m.Appearance)
	textTheme(im, 240, 114, 3, now.Format("15:04"), p.PrimaryText, true, m.Appearance)
	textTheme(im, 240, 144, 2, "APPLICATIONS", p.SecondaryText, true, m.Appearance)
	n := m.CarouselCount()
	focus := m.Selected % n
	for _, entry := range []struct{ x, idx int }{{63, (focus + n - 1) % n}, {417, (focus + 1) % n}} {
		app := m.AppCards[entry.idx]
		rect(im, entry.x-33, 211, entry.x+33, 296, p.Surface)
		textTheme(im, entry.x, 237, 2, app.Icon, p.SecondaryText, true, m.Appearance)
		textTheme(im, entry.x, 263, 1, "APP", p.SecondaryText, true, m.Appearance)
	}
	app := m.AppCards[focus]
	rect(im, 120, 172, 360, 349, p.Surface)
	outlineRect(im, 118, 170, 362, 351, p.Focus)
	disc(im, 240, 238, 42, p.Selected)
	textTheme(im, 240, 227, 3, app.Icon, p.Accent, true, m.Appearance)
	textTheme(im, 240, 294, 3, app.Title, p.PrimaryText, true, m.Appearance)
	detail := "SELECT TO OPEN"
	if focus == 0 {
		detail = "TIME & ALARMS"
	}
	if focus == 1 {
		detail = "COUNTDOWN"
	}
	if focus == 2 {
		detail = "WI-FI & SYSTEM"
	}
	textTheme(im, 240, 321, 1, detail, p.SecondaryText, true, m.Appearance)
	// Fixed three-dot window represents position without overflowing a round screen.
	for i := -1; i <= 1; i++ {
		c := p.Border
		if i == 0 {
			c = p.Accent
		}
		disc(im, 240+i*17, 368, 5, c)
	}
	button(im, 240, 403, 172, 42, "TOUCH TEST", p.Surface, p.PrimaryText)
}
