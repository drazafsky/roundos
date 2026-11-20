// Package gesture defines the public RoundOS input gesture payloads.
// Coordinates and deltas are expressed in 480x480 physical display pixels.
package gesture

type Event struct {
	Type      string
	X, Y      float64
	DX, DY    float64
	Scale     float64
	Angle     float64 // degrees [-180, 180]
	Fingers   int
	Direction string
}
