package textedit

import "testing"

func TestRuneEditAndDraft(t *testing.T) {
	s := New("Aé", false)
	s.Move(-1)
	s.Insert("世")
	if s.Draft != "A世é" || s.Cursor != 2 {
		t.Fatalf("%+v", s)
	}
	s.Hide()
	if !s.Dirty() || s.Visible == true {
		t.Fatal("hiding lost draft")
	}
	s.Show()
	s.Cancel()
	if s.Draft != "Aé" || s.Dirty() {
		t.Fatal("cancel")
	}
	s.Show()
	s.Insert("!")
	s.Save()
	if s.Committed != "Aé!" {
		t.Fatal("save")
	}
}
func TestSecureClear(t *testing.T) {
	s := New("secret", true)
	if s.Masked() != "******" {
		t.Fatal("mask")
	}
	s.ClearSensitive()
	if s.Draft != "" || s.Committed != "" {
		t.Fatal("sensitive text retained")
	}
}
func TestWheelCommands(t *testing.T) {
	s := New("", false)
	if len(s.WheelItems()) < 30 {
		t.Fatal("wheel missing actions")
	}
	s.Rotate(-1)
	if s.WheelLabel() != Done {
		t.Fatalf("%s", s.WheelLabel())
	}
	s.Rotate(1)
	if s.WheelLabel() != "A" {
		t.Fatal("wrap")
	}
	if done, _ := s.ActivateWheel(); done || s.Draft != "a" {
		t.Fatal("insert")
	}
	s.Command(CursorLeft)
	s.Command("z")
	if s.Draft != "za" {
		t.Fatal(s.Draft)
	}
	done, _ := s.Command(Done)
	if !done || s.Dirty() {
		t.Fatal("save")
	}
}
func TestCursorAndLimits(t *testing.T) {
	s := New("éxy", false)
	s.SetCursorFromTap(0)
	if s.Erase() {
		t.Fatal("erase before start")
	}
	s.SetCursorFromTap(1)
	s.Move(10)
	if s.Cursor != 3 {
		t.Fatal(s.Cursor)
	}
	s.Insert("!")
	if s.Draft != "éxy!" {
		t.Fatal(s.Draft)
	}
}
