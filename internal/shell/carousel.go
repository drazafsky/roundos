package shell

import (
	"fmt"
	"image"
	"math"
	"strings"
	"time"
)

const carouselDuration = 220 * time.Millisecond
const carouselFrame = 16 * time.Millisecond

// moveCarousel keeps an unwrapped visual position so crossing either end slides
// one card, and a new input can continue from the current on-screen position.
func (m *Model) moveCarousel(delta int, now time.Time) bool {
	n := m.CarouselCount()
	step := delta % n
	next := (m.Selected + step + n) % n
	if !m.Appearance.CarouselWrap {
		next = m.Selected + max(-m.Selected, min(n-1-m.Selected, delta))
		step = next - m.Selected
	}
	if next == m.Selected {
		return false
	}
	position := m.carouselPosition(now)
	target := float64(m.Selected)
	if !m.carouselStarted.IsZero() && now.Before(m.carouselStarted.Add(carouselDuration)) {
		target = m.carouselTarget
	}
	m.Selected = next
	m.carouselStarted = time.Time{}
	if !m.Appearance.ReduceMotion {
		m.carouselFrom = position
		m.carouselTarget = target + float64(step)
		m.carouselStarted = now
		m.carouselNextFrame = now.Add(carouselFrame)
	}
	return true
}

func (m *Model) carouselPosition(now time.Time) float64 {
	if !m.carouselAnimating(now) {
		return float64(m.Selected)
	}
	t := max(0, float64(now.Sub(m.carouselStarted))/float64(carouselDuration))
	// Smoothstep gives a continuous slide with stationary endpoints.
	t = t * t * (3 - 2*t)
	return m.carouselFrom + (m.carouselTarget-m.carouselFrom)*t
}

func (m *Model) carouselAnimating(now time.Time) bool {
	return !m.Appearance.ReduceMotion && !m.carouselStarted.IsZero() && now.Before(m.carouselStarted.Add(carouselDuration))
}

// AdvanceCarousel requests the final frame as well as intermediate ones. Drawing
// itself never mutates the model, including snapshots used by the renderer.
func (m *Model) AdvanceCarousel(now time.Time) bool {
	if m.carouselStarted.IsZero() {
		return false
	}
	if m.Page != Home || m.Appearance.ReduceMotion || !now.Before(m.carouselStarted.Add(carouselDuration)) {
		m.carouselStarted = time.Time{}
		return true
	}
	if now.Before(m.carouselNextFrame) {
		return false
	}
	m.carouselNextFrame = now.Add(carouselFrame)
	return true
}

func (m *Model) NextCarouselWake(now time.Time) time.Duration {
	if m.Page != Home || m.Appearance.ReduceMotion || m.carouselStarted.IsZero() {
		return 0
	}
	deadline := m.carouselNextFrame
	if end := m.carouselStarted.Add(carouselDuration); end.Before(deadline) {
		deadline = end
	}
	return max(time.Nanosecond, deadline.Sub(now))
}

func launcherHeader(now time.Time) (clock, date string) {
	local := now.Local()
	return local.Format("15:04"), local.Format("Jan. 2, 2006")
}

// Each installed app has an icon and a name beneath it. Only the focused entry
// launches; neighboring entries remain focus targets for touch and rotary.
func drawCarousel(im *image.RGBA, m *Model, now time.Time) {
	p := m.Appearance.Palette()
	clock, date := launcherHeader(now)
	textTheme(im, 240, 79, 3, clock, p.Accent, true, m.Appearance)
	textTheme(im, 240, 114, 3, strings.ToUpper(date), p.PrimaryText, true, m.Appearance)
	textTheme(im, 240, 144, 2, "APPLICATIONS", p.SecondaryText, true, m.Appearance)

	n := m.CarouselCount()
	position := m.carouselPosition(now)
	for slot := int(math.Floor(position)) - 2; slot <= int(math.Ceil(position))+2; slot++ {
		if !m.Appearance.CarouselWrap && (slot < 0 || slot >= n) {
			continue
		}
		offset := float64(slot) - position
		x := 240 + int(math.Round(offset*177))
		if x < 20 || x > 460 {
			continue
		}
		weight := max(0, 1-math.Abs(offset))
		idx := (slot%n + n) % n
		app := m.AppCards[idx]
		foreground, fill := p.SecondaryText, p.Surface
		if weight > 0.5 {
			foreground, fill = p.Accent, p.Selected
		}
		radius := 26 + int(math.Round(16*weight))
		disc(im, x, 238, radius, fill)
		if idx == m.Selected {
			ring(im, x, 238, radius+5, 2, p.Focus)
		}
		iconRadius := 18 + int(math.Round(10*weight))
		if !drawAppIcon(im, app, x, 238, iconRadius, foreground) {
			textTheme(im, x, 231, 2, app.Icon, foreground, true, m.Appearance)
		}
		labelColor := p.SecondaryText
		if weight > 0.5 {
			labelColor = p.PrimaryText
		}
		size := 1 + int(math.Round(2*weight))
		if m.Appearance.TextScale >= 125 {
			size++
		}
		// Fit labels to both their card and the round screen's safe chord.
		width := min(100+int(120*weight), 2*min(x-38, 442-x))
		if width < 6 {
			continue
		}
		for size > 1 && (len([]rune(app.Title))*6-1)*size > width {
			size--
		}
		for i, line := range wrapDetail(app.Title, max(1, width/(6*size)), 2) {
			text(im, x, 294+i*(7*size+4), size, line, labelColor, true)
		}
	}
	textTheme(im, 240, 355, 1, "SELECT TO OPEN", p.SecondaryText, true, m.Appearance)
	textTheme(im, 240, 374, 1, fmt.Sprintf("%d / %d", m.Selected+1, n), p.SecondaryText, true, m.Appearance)
}
