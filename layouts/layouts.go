// Package layouts embeds the built-in applications' compiled presentation.
package layouts

import _ "embed"

//go:generate go run ../cmd/roundui compile -o clock.rui.bin clock.rui.yaml
//go:generate go run ../cmd/roundui compile -o timer.rui.bin timer.rui.yaml

//go:embed clock.rui.bin
var Clock []byte

//go:embed timer.rui.bin
var Timer []byte
