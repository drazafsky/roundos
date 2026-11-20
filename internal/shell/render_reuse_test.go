package shell

import (
	"image"
	"testing"
	"time"
)

func TestDrawIntoReusesBackingPixels(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, Width, Height))
	backing := &img.Pix[0]
	DrawInto(img, New(), time.Date(2026, 11, 10, 13, 0, 0, 0, time.UTC))
	DrawInto(img, New(), time.Date(2026, 11, 10, 13, 1, 0, 0, time.UTC))
	if &img.Pix[0] != backing {
		t.Fatal("framebuffer was reallocated")
	}
}
