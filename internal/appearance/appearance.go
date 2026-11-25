// Package appearance manages settings and semantic color tokens. No RoundOS
// widget or app is required to hardcode foreground or accent colors.
package appearance

import (
	"encoding/json"
	"errors"
	"image/color"
	"os"
	"path/filepath"
)

const (
	Dark     = "dark"
	Light    = "light"
	Contrast = "contrast"
)

type Settings struct {
	Theme        string `json:"theme"`
	TextScale    int    `json:"text_scale"`
	ReduceMotion bool   `json:"reduce_motion"`
	LargeTargets bool   `json:"large_targets"`
	CarouselWrap bool   `json:"carousel_wrap"`
}

func Default() Settings { return Settings{Theme: Dark, TextScale: 100, CarouselWrap: true} }
func (s Settings) Valid() error {
	if s.Theme != Dark && s.Theme != Light && s.Theme != Contrast {
		return errors.New("invalid theme")
	}
	if s.TextScale != 100 && s.TextScale != 125 && s.TextScale != 150 {
		return errors.New("invalid text scale")
	}
	return nil
}
func Load(path string) Settings {
	d := Default()
	b, err := os.ReadFile(path)
	if err != nil {
		return d
	}
	if err = json.Unmarshal(b, &d); err != nil || d.Valid() != nil {
		return Default()
	}
	return d
}
func Save(path string, s Settings) error {
	if err := s.Valid(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Colors are semantic tokens and can be extended without changing apps.
type Colors struct{ Background, Surface, Selected, PrimaryText, SecondaryText, Accent, OnAccent, Focus, Border, Disabled, Success, Warning, Danger color.RGBA }

func RGB(r, g, b uint8) color.RGBA { return color.RGBA{R: r, G: g, B: b, A: 255} }
func (s Settings) Palette() Colors {
	switch s.Theme {
	case Light:
		return Colors{RGB(237, 243, 246), RGB(255, 255, 255), RGB(218, 243, 239), RGB(22, 43, 59), RGB(83, 111, 125), RGB(8, 127, 112), RGB(255, 255, 255), RGB(8, 127, 112), RGB(173, 193, 204), RGB(147, 157, 161), RGB(12, 112, 65), RGB(150, 93, 0), RGB(172, 42, 42)}
	case Contrast:
		return Colors{RGB(0, 0, 0), RGB(16, 16, 16), RGB(35, 35, 35), RGB(255, 255, 255), RGB(250, 250, 250), RGB(0, 255, 255), RGB(0, 0, 0), RGB(255, 255, 0), RGB(255, 255, 255), RGB(180, 180, 180), RGB(0, 255, 0), RGB(255, 255, 0), RGB(255, 80, 80)}
	default:
		return Colors{RGB(11, 20, 33), RGB(35, 58, 78), RGB(36, 76, 82), RGB(244, 249, 255), RGB(163, 186, 202), RGB(69, 217, 195), RGB(9, 43, 40), RGB(69, 217, 195), RGB(59, 83, 103), RGB(99, 115, 129), RGB(48, 203, 133), RGB(240, 188, 112), RGB(255, 104, 107)}
	}
}
func (s *Settings) NextTheme() {
	switch s.Theme {
	case Dark:
		s.Theme = Light
	case Light:
		s.Theme = Contrast
	default:
		s.Theme = Dark
	}
}
func (s *Settings) NextScale() {
	switch s.TextScale {
	case 100:
		s.TextScale = 125
	case 125:
		s.TextScale = 150
	default:
		s.TextScale = 100
	}
}
