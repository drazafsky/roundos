package appearance

import (
	"path/filepath"
	"testing"
)

func TestThemeAndSave(t *testing.T) {
	s := Default()
	dark := s.Palette()
	s.NextTheme()
	if s.Theme != Light || dark.Background == s.Palette().Background {
		t.Fatal("theme didn't change")
	}
	s.NextTheme()
	if s.Theme != Contrast {
		t.Fatal("contrast")
	}
	s.NextTheme()
	s.NextScale()
	s.ReduceMotion = true
	p := filepath.Join(t.TempDir(), "prefs.json")
	if err := Save(p, s); err != nil {
		t.Fatal(err)
	}
	r := Load(p)
	if r != s {
		t.Fatalf("%+v != %+v", r, s)
	}
}
