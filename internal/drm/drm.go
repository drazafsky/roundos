//go:build drm && linux && cgo

package drm

/*
#cgo pkg-config: libdrm
#include <stdlib.h>
#include "drm_bridge.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"image"
	"unsafe"
)

type Display struct{ ptr *C.RoundDisplay }

func Open(device, connector string) (*Display, error) {
	d := C.CString(device)
	c := C.CString(connector)
	defer C.free(unsafe.Pointer(d))
	defer C.free(unsafe.Pointer(c))
	var message [512]C.char
	result := C.round_open(d, c, &message[0], C.size_t(len(message)))
	if result == nil {
		return nil, errors.New(C.GoString(&message[0]))
	}
	return &Display{ptr: result}, nil
}

func (d *Display) Present(img *image.RGBA) error {
	if d == nil || d.ptr == nil {
		return errors.New("DRM display is closed")
	}
	if img.Bounds() != image.Rect(0, 0, 480, 480) || img.Stride != 480*4 {
		return fmt.Errorf("expected contiguous RGBA 480x480 image, got %v stride %d", img.Bounds(), img.Stride)
	}
	var message [512]C.char
	if C.round_present(d.ptr, (*C.uchar)(unsafe.Pointer(&img.Pix[0])), &message[0], C.size_t(len(message))) < 0 {
		return errors.New(C.GoString(&message[0]))
	}
	return nil
}
func (d *Display) Close() error {
	if d != nil && d.ptr != nil {
		C.round_close(d.ptr)
		d.ptr = nil
	}
	return nil
}
