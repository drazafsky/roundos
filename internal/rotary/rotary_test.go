package rotary

import "testing"

func TestRelativeDetentsAndReverse(t *testing.T) {
	d := NewDecoder(DefaultConfig())
	if len(d.Feed(evRel, RelDial, 3)) != 0 {
		t.Fatal("should batch until SYN")
	}
	ev := d.Feed(evSyn, synReport, 0)
	if len(ev) != 1 || ev[0].Kind != "rotate" || ev[0].Delta != 3 {
		t.Fatal(ev)
	}
	cfg := DefaultConfig()
	cfg.Reverse = true
	d = NewDecoder(cfg)
	d.Feed(evRel, RelDial, 2)
	ev = d.Feed(evSyn, synReport, 0)
	if len(ev) != 1 || ev[0].Delta != -2 {
		t.Fatal(ev)
	}
}
func TestKeysButtonAndRepeat(t *testing.T) {
	d := NewDecoder(DefaultConfig())
	ev := d.Feed(evKey, KeyEnter, 1)
	if len(ev) != 1 || ev[0].Kind != "button_down" {
		t.Fatal(ev)
	}
	if len(d.Feed(evKey, KeyEnter, 2)) != 0 {
		t.Fatal("repeat triggered event")
	}
	ev = d.Feed(evKey, KeyEnter, 0)
	if len(ev) != 1 || ev[0].Kind != "button_up" {
		t.Fatal(ev)
	}
	d.Feed(evKey, KeyUp, 1)
	ev = d.Feed(evSyn, synReport, 0)
	if len(ev) != 1 || ev[0].Delta != -1 {
		t.Fatal(ev)
	}
	d.Feed(evKey, KeyDown, 1)
	ev = d.Feed(evSyn, synReport, 0)
	if len(ev) != 1 || ev[0].Delta != 1 {
		t.Fatal(ev)
	}
}
