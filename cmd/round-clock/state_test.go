package main

import (
	"strings"
	"testing"
)

func TestClockLabelRetainsEditorCapacity(t *testing.T) {
	s := &clockState{}
	for _, label := range []string{strings.Repeat("x", 256), strings.Repeat("é", 256), strings.Repeat("🌍", 256), ""} {
		if err := s.setLabel(label); err != nil || s.label != label {
			t.Fatalf("editor-supported label rejected: %v", err)
		}
	}
	s.label = "previous"
	for _, label := range []string{strings.Repeat("x", 257), "\xff"} {
		if err := s.setLabel(label); err == nil || s.label != "previous" {
			t.Fatal("invalid result altered label")
		}
	}
}
