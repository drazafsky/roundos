//go:build !drm

package drm

import (
	"errors"
	"image"
)

type Display struct{}

func Open(device, connector string) (*Display, error) {
	return nil, errors.New("DRM backend not compiled: build with CGO_ENABLED=1 go build -tags drm")
}
func (d *Display) Present(img *image.RGBA) error { return errors.New("DRM backend unavailable") }
func (d *Display) Close() error                  { return nil }
