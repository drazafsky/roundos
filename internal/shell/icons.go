package shell

import (
	"image"
	"image/color"
	"math"
)

// drawAppIcon uses the current semantic foreground supplied by the caller.
// Built-in app identities select symbols; optional apps retain their initials.
func drawAppIcon(im *image.RGBA, app AppCard, x, y, radius int, foreground color.RGBA) bool {
	stroke := max(1, radius/12)
	line := func(x1, y1, x2, y2 int) {
		iconLine(im, x+x1, y+y1, x+x2, y+y2, stroke, foreground)
	}
	switch app.Page {
	case Clock:
		ring(im, x, y, radius-2, stroke*2, foreground)
		for _, tick := range [][4]int{{0, -radius + 4, 0, -radius + 8}, {radius - 4, 0, radius - 8, 0}, {0, radius - 4, 0, radius - 8}, {-radius + 4, 0, -radius + 8, 0}} {
			line(tick[0], tick[1], tick[2], tick[3])
		}
		line(0, 0, -radius/3, -radius/3)
		line(0, 0, radius/2, -radius/2)
		disc(im, x, y, stroke+1, foreground)
	case Timer:
		bodyY, bodyRadius := radius/5, radius*3/4
		ring(im, x, y+bodyY, bodyRadius, stroke*2, foreground)
		line(0, -radius+2, 0, bodyY-bodyRadius)
		line(-radius/5, -radius+2, radius/5, -radius+2)
		line(radius/2, -radius/2, radius*2/3, -radius*2/3)
		line(radius/2, -radius*2/3, radius*2/3, -radius/2)
		line(0, bodyY, 0, bodyY-bodyRadius/2)
		line(0, bodyY, bodyRadius/3, bodyY+bodyRadius/3)
		disc(im, x, y+bodyY, stroke+1, foreground)
	case Settings:
		// Eight teeth, outlined with an open center so the same symbol remains
		// clear on both a selected launcher disc and a plain switcher row.
		var points [32]image.Point
		for i := range points {
			r := float64(radius - 2)
			if i%4 == 0 || i%4 == 3 {
				r *= 0.76
			}
			a := float64(i)*math.Pi/16 - math.Pi/32
			points[i] = image.Pt(int(math.Round(r*math.Cos(a))), int(math.Round(r*math.Sin(a))))
		}
		for i, point := range points {
			next := points[(i+1)%len(points)]
			line(point.X, point.Y, next.X, next.Y)
		}
		ring(im, x, y, radius/3, stroke*2, foreground)
	default:
		return false
	}
	return true
}

// iconLine draws a short round-ended stroke without allocating a path/image.
func iconLine(im *image.RGBA, x1, y1, x2, y2, radius int, foreground color.RGBA) {
	dx, dy := x2-x1, y2-y1
	steps := max(abs(dx), abs(dy))
	if steps == 0 {
		disc(im, x1, y1, radius, foreground)
		return
	}
	for i := 0; i <= steps; i++ {
		x := x1 + int(math.Round(float64(dx*i)/float64(steps)))
		y := y1 + int(math.Round(float64(dy*i)/float64(steps)))
		disc(im, x, y, radius, foreground)
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
