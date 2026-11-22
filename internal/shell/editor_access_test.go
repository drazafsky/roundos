package shell

import (
	"image/color"
	"testing"
	"time"

	"roundos/internal/appearance"
	"roundos/internal/wire"
)

func TestTextTouchSaveAndEditorReturn(t *testing.T) {
	m := New()
	m.Select(Clock)
	if !m.OpenAppEditor(wire.ClockID, wire.TextRequest{ID: "label", Initial: "Hi"}) {
		t.Fatal("app editor not opened")
	}
	// Q is the first row's leftmost key, and space is the bottom middle.
	if changed, _ := m.Tap(61, 218); !changed || m.Editor.Draft != "Hiq" {
		t.Fatalf("touch key: %s", m.Editor.Draft)
	}
	if changed, _ := m.Tap(200, 364); !changed || m.Editor.Draft != "Hiq " {
		t.Fatalf("space: %q", m.Editor.Draft)
	}
	if changed, _ := m.Tap(319, 362); !changed || m.Editor.Draft != "Hiq" {
		t.Fatal("backspace")
	}
	if changed, _ := m.Tap(240, 413); !changed || m.Page != Clock {
		t.Fatal("done")
	}
	id, result := m.TakeTextResult()
	if id != wire.ClockID || result == nil || result.ID != "label" || result.Canceled || result.Value != "Hiq" {
		t.Fatalf("result %+v %q", result, id)
	}
	if _, result = m.TakeTextResult(); result != nil {
		t.Fatal("result delivered twice")
	}
}
func TestDraftBackKeepDiscard(t *testing.T) {
	m := New()
	m.Select(Clock)
	m.OpenAppEditor(wire.ClockID, wire.TextRequest{ID: "f", Initial: "A"})
	m.Editor.Insert("b")
	if !m.Back() || m.Page != TextEditor || m.Editor.Visible || !m.Editor.Dirty() {
		t.Fatal("first Back should hide keyboard without losing draft")
	}
	if !m.Back() || m.Page != DiscardPrompt {
		t.Fatal("second Back should ask to discard")
	}
	if !m.ResolveDiscard(false) || m.Page != TextEditor || !m.Editor.Visible || m.Editor.Draft != "Ab" {
		t.Fatal("keep editing should restore draft")
	}
	m.Back()
	m.Back()
	if !m.ResolveDiscard(true) || m.Page != Clock {
		t.Fatal("discard should return to parent")
	}
	_, r := m.TakeTextResult()
	if r == nil || !r.Canceled || r.Value != "" {
		t.Fatalf("unsafe discard result %+v", r)
	}
}
func TestRotaryOnlyTextEditing(t *testing.T) {
	m := New()
	m.OpenEditor()
	_, action, delta := m.Rotate(1)
	if action != "" || delta != 0 {
		t.Fatal("wheel move forwarded to app")
	}
	if changed, _ := m.Activate(); !changed || m.Editor.Draft != "b" {
		t.Fatalf("rotary char: %q", m.Editor.Draft)
	}
	// The command wheel always exposes Done even without touch.
	m.Editor.Rotate(-1 - 1) // b -> Done
	if m.Editor.WheelLabel() != "DONE" {
		t.Fatalf("wheel %s", m.Editor.WheelLabel())
	}
	if changed, _ := m.Activate(); !changed || m.Page != Home || m.Editor.Committed != "b" {
		t.Fatal("rotary Done")
	}
}
func TestSecureEditorClearsOnShellInterrupt(t *testing.T) {
	m := New()
	m.Select(Clock)
	m.OpenAppEditor(wire.ClockID, wire.TextRequest{ID: "password", Initial: "private", Secure: true})
	if !m.OpenNotifications() {
		t.Fatal("notification should open")
	}
	if m.Editor.Draft != "" || m.Editor.Committed != "" {
		t.Fatal("secure values survived foreground loss")
	}
	app, result := m.TakeTextResult()
	if app != wire.ClockID || result == nil || !result.Canceled || result.Value != "" {
		t.Fatalf("result: %s %+v", app, result)
	}
}

func TestNonsecureEditorInterruptCancelsRequestBeforeReopening(t *testing.T) {
	m := New()
	m.Select(Clock)
	request := wire.TextRequest{ID: "title", Initial: "Original"}
	if !m.OpenAppEditor(wire.ClockID, request) {
		t.Fatal("initial editor did not open")
	}
	m.Editor.Insert(" draft")
	m.OpenNotifications()
	app, result := m.TakeTextResult()
	if app != wire.ClockID || result == nil || result.ID != "title" || !result.Canceled || result.Value != "" {
		t.Fatalf("interrupted nonsecure request not canceled: %s %+v", app, result)
	}
	if _, result := m.TakeTextResult(); result != nil {
		t.Fatal("canceled result repeated")
	}
	m.Select(Clock)
	if !m.OpenAppEditor(wire.ClockID, request) || m.Editor.Draft != "Original" {
		t.Fatal("editor did not reopen with authoritative app value")
	}
}
func TestAppearanceRecolorsAllShellViews(t *testing.T) {
	m := New()
	m.Select(Clock)
	m.SetView(wire.ClockID, wire.View{Title: "CLOCK", Primary: "12:34"})
	dark := Draw(m, time.Now()).RGBAAt(240, 45)
	s := m.Appearance
	s.Theme = appearance.Light
	m.SetAppearance(s)
	light := Draw(m, time.Now()).RGBAAt(240, 45)
	if dark == light || light != (color.RGBA{R: 237, G: 243, B: 246, A: 255}) {
		t.Fatalf("background unchanged: %v %v", dark, light)
	}
	m.Select(Accessibility)
	m.AccessFocus = 0
	m.Activate()
	if m.Appearance.Theme != appearance.Contrast || !m.AppearanceDirty {
		t.Fatal("rotary theme setting")
	}
	if Draw(m, time.Now()).Bounds().Dx() != 480 {
		t.Fatal("frame")
	}
}
func TestSwitcherIncludesEditorAndAccessibility(t *testing.T) {
	m := New()
	m.Select(Clock)
	m.OpenSwitcher()
	m.Rotate(3)
	if m.SwitcherFocus != 3 {
		t.Fatal("rotary switcher did not reach editor")
	}
	if changed, _ := m.Activate(); !changed || m.Page != TextEditor {
		t.Fatal("switcher editor activation")
	}
	m.Back()
	m.Back() // not dirty, return to switcher
	if m.Page != Switcher {
		t.Fatal("editor did not return to switcher")
	}
	m.Rotate(1)
	if m.SwitcherFocus != 4 {
		t.Fatal("rotary switcher cannot reach accessibility")
	}
	if changed, _ := m.Activate(); !changed || m.Page != Accessibility {
		t.Fatal("switcher accessibility activation")
	}
	if !m.Back() || m.Page != Switcher || m.SwitcherFocus != 4 {
		t.Fatal("Back failed to restore selected switcher item")
	}

}
