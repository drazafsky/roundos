package appearance

import (
	"os"
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

func TestCarouselWrapDefaultMigrationAndPersistence(t *testing.T) {
	p := filepath.Join(t.TempDir(), "appearance.json")
	if err := os.WriteFile(p, []byte(`{"theme":"light","text_scale":125,"reduce_motion":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	s := Load(p)
	if !s.CarouselWrap || s.Theme != Light || s.TextScale != 125 || !s.ReduceMotion {
		t.Fatalf("existing preferences did not gain the default without losing settings: %+v", s)
	}
	s.CarouselWrap = false
	if err := Save(p, s); err != nil {
		t.Fatal(err)
	}
	if got := Load(p); got != s {
		t.Fatalf("disabled wrapping did not survive a restart: %+v", got)
	}
}
