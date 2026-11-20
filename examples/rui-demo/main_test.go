package main

import (
	"strings"
	"testing"
)

func TestTitleEditsPreserveValidState(t *testing.T) {
	for _, test := range []struct {
		name  string
		value string
		valid bool
	}{
		{"ASCII boundary", strings.Repeat("a", 240), true},
		{"ASCII overflow", strings.Repeat("a", 241), false},
		{"Unicode boundary", strings.Repeat("界", 80), true},
		{"Unicode overflow", strings.Repeat("界", 81), false},
		{"invalid UTF-8", string([]byte{0xff}), false},
		{"empty", "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			title := "Previous valid title"
			err := setTitle(&title, test.value)
			if (err == nil) != test.valid {
				t.Fatalf("setTitle returned %v, valid=%t", err, test.valid)
			}
			if test.valid {
				if title != test.value {
					t.Fatalf("accepted title was not saved: %q", title)
				}
			} else if title != "Previous valid title" {
				t.Fatalf("rejected edit replaced the last valid title: %q", title)
			}
			if err := setTitle(&title, "A later valid edit"); err != nil || title != "A later valid edit" {
				t.Fatalf("subsequent edit failed: %q, %v", title, err)
			}
		})
	}
}
