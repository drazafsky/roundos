package notify

import (
	"testing"
	"time"
)

func TestAdaptiveAndGroup(t *testing.T) {
	s := New()
	at := time.Unix(100, 0)
	q := s.Emit("weather", Request{Title: "Forecast updated", Priority: Quiet}, at, false)
	if q.Presentation != Silent {
		t.Fatal(q)
	}
	a := s.Emit("timer", Request{Title: "Timer finished", Priority: Actionable, Action: "open"}, at, false)
	if a.Presentation != Banner {
		t.Fatal(a)
	}
	dup := s.Emit("timer", Request{Title: "Timer finished", Priority: Actionable, Action: "open"}, at.Add(2*time.Second), false)
	if dup.New || dup.Presentation != Silent || dup.Item.Count != 2 || len(s.Items()) != 2 {
		t.Fatal(dup)
	}
	s.SetQuietMode(true)
	if r := s.Emit("timer", Request{Title: "Timer finished again", Priority: Actionable}, at, false); r.Presentation != Silent {
		t.Fatal(r)
	}
	s.SetQuietMode(false)
	if r := s.Emit("untrusted", Request{Title: "Danger", Priority: Critical}, at, false); r.Presentation != Banner || r.Item.Request.Priority != Actionable {
		t.Fatal(r)
	}
	if r := s.Emit("system", Request{Title: "Critical", Priority: Critical}, at, true); r.Presentation != Modal {
		t.Fatal(r)
	}
}
func TestMutationsBoundedAndSanitized(t *testing.T) {
	s := New()
	s.SetAppPriority("weather", Quiet)
	if p := s.Emit("weather", Request{Title: "Wind", Priority: Actionable}, time.Now(), false).Presentation; p != Silent {
		t.Fatal(p)
	}
	for i := 0; i < 80; i++ {
		s.Emit("timer", Request{Title: "New", Group: string(rune('A' + i))}, time.Unix(int64(i*40), 0), false)
	}
	if len(s.Items()) != 60 {
		t.Fatal("unbounded", len(s.Items()))
	}
	id := s.Items()[0].ID
	if !s.MarkRead(id) || !s.Items()[0].Read || !s.Dismiss(id) {
		t.Fatal("mutation")
	}
	if s.Dismiss(id) {
		t.Fatal("duplicate dismiss")
	}
	s.Clear()
	if len(s.Items()) != 0 {
		t.Fatal("clear")
	}
}
