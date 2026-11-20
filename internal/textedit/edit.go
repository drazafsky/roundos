// Package textedit is a display-independent Unicode text editor for RoundOS.
// The shell owns display, focus, input and discard confirmation. This package
// never persists secure field contents.
package textedit

import (
	"strings"
	"unicode/utf8"
)

const MaxRunes = 256

// Commands are accessible from a rotary wheel without touch.
const (
	Backspace   = "BKSP"
	Space       = "SPACE"
	CursorLeft  = "LEFT"
	CursorRight = "RIGHT"
	Shift       = "SHIFT"
	NextGroup   = "GROUP"
	Done        = "DONE"
)

var groupNames = []string{"ABC", "123", "SYM"}
var groups = []string{"abcdefghijklmnopqrstuvwxyz", "0123456789", ".,!?@#-$%&+/=:;()_"}
var commands = []string{Space, Backspace, CursorLeft, CursorRight, Shift, NextGroup, Done}

// State holds a nonsecret editable draft. There is deliberately no automatic
// serialization or recovery for secure fields.
type State struct {
	Committed string
	Draft     string
	Cursor    int // rune position
	Group     int
	Wheel     int
	Upper     bool
	Visible   bool
	Secure    bool
}

func New(value string, secure bool) State {
	value = clamp(value)
	return State{Committed: value, Draft: value, Cursor: utf8.RuneCountInString(value), Visible: true, Secure: secure}
}
func clamp(s string) string {
	r := []rune(s)
	if len(r) > MaxRunes {
		return string(r[:MaxRunes])
	}
	return s
}
func (s *State) Dirty() bool { return s.Draft != s.Committed }
func (s *State) Cancel() {
	s.Draft = s.Committed
	s.Cursor = utf8.RuneCountInString(s.Draft)
	s.Visible = false
}
func (s *State) Save() { s.Committed = s.Draft; s.Visible = false }
func (s *State) Hide() { s.Visible = false }
func (s *State) Show() { s.Visible = true }
func (s *State) ClearSensitive() {
	if s.Secure {
		s.Committed = ""
		s.Draft = ""
		s.Cursor = 0
		s.Visible = false
	}
}
func (s *State) Insert(v string) bool {
	r := []rune(s.Draft)
	ins := []rune(v)
	if len(r)+len(ins) > MaxRunes {
		return false
	}
	s.Cursor = clampCursor(s.Cursor, len(r))
	next := make([]rune, 0, len(r)+len(ins))
	next = append(next, r[:s.Cursor]...)
	next = append(next, ins...)
	next = append(next, r[s.Cursor:]...)
	s.Draft = string(next)
	s.Cursor += len(ins)
	return true
}
func (s *State) Erase() bool {
	r := []rune(s.Draft)
	s.Cursor = clampCursor(s.Cursor, len(r))
	if s.Cursor == 0 {
		return false
	}
	s.Draft = string(append(r[:s.Cursor-1], r[s.Cursor:]...))
	s.Cursor--
	return true
}
func (s *State) Move(delta int) bool {
	n := clampCursor(s.Cursor+delta, utf8.RuneCountInString(s.Draft))
	if n == s.Cursor {
		return false
	}
	s.Cursor = n
	return true
}
func clampCursor(n, max int) int {
	if n < 0 {
		return 0
	}
	if n > max {
		return max
	}
	return n
}
func (s *State) SetCursorFromTap(fraction float64) {
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}
	s.Cursor = int(float64(utf8.RuneCountInString(s.Draft))*fraction + 0.5)
}
func (s *State) SelectGroup(g int) {
	if g >= 0 && g < len(groups) {
		s.Group = g
		s.Wheel = 0
	}
}
func (s *State) WheelItems() []string {
	r := []rune(groups[s.Group])
	items := make([]string, 0, len(r)+len(commands))
	for _, v := range r {
		items = append(items, string(v))
	}
	return append(items, commands...)
}
func (s *State) WheelLabel() string {
	items := s.WheelItems()
	if len(items) == 0 {
		return ""
	}
	return strings.ToUpper(items[clampCursor(s.Wheel, len(items)-1)])
}
func (s *State) Rotate(d int)                             { n := len(s.WheelItems()); s.Wheel = ((s.Wheel+d)%n + n) % n }
func (s *State) ActivateWheel() (done bool, changed bool) { return s.Command(s.WheelItems()[s.Wheel]) }
func (s *State) Command(command string) (done, changed bool) {
	switch command {
	case Backspace:
		return false, s.Erase()
	case Space:
		return false, s.Insert(" ")
	case CursorLeft:
		return false, s.Move(-1)
	case CursorRight:
		return false, s.Move(1)
	case Shift:
		s.Upper = !s.Upper
		return false, true
	case NextGroup:
		s.SelectGroup((s.Group + 1) % len(groups))
		return false, true
	case Done:
		s.Save()
		return true, true
	default:
		r := []rune(command)
		if len(r) != 1 {
			return false, false
		}
		v := command
		if s.Upper {
			v = strings.ToUpper(v)
		}
		return false, s.Insert(v)
	}
}
func (s *State) Masked() string {
	if s.Secure {
		return strings.Repeat("*", utf8.RuneCountInString(s.Draft))
	}
	return s.Draft
}
