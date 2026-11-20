package systemgestures

import (
	"testing"
	"time"

	"roundos/internal/gesture"
	"roundos/internal/shell"
	"roundos/internal/touch"
)

// This feeds the recognizer realistic type-B touchscreen snapshots.  Separate
// contacts can be updated in consecutive SYN_REPORT frames on the HyperPixel.
func TestStaggeredTwoFingerSwipesReachShell(t *testing.T) {
	for _, tc := range []struct {
		name     string
		dx, dy   int
		initial  int
		expected int
	}{
		{"next application", -1, 0, shell.Clock, shell.Timer},
		{"previous application", 1, 0, shell.Timer, shell.Clock},
		// The physically upward two-finger swipe arrived with positive raw Y
		// movement on the tested panel. The system router compensates without
		// flipping normal tap coordinates.
		{"open launcher", 0, 1, shell.Clock, shell.Home},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			cfg.TwoSwitch = true
			cfg.TwoLauncher = true
			router := New(cfg)
			d := gesture.New()
			model := shell.New()
			model.Select(tc.initial)
			base := time.Unix(1000, 0)
			send := func(ms int, contacts ...touch.Contact) {
				evs := d.Process(touch.Frame{At: base.Add(time.Duration(ms) * time.Millisecond), Contacts: contacts})
				for _, ev := range evs {
					switch router.Handle(ev) {
					case NextApp:
						model.NextRunning(1)
					case PreviousApp:
						model.NextRunning(-1)
					case Launcher:
						model.Select(shell.Home)
					}
				}
			}
			a, b := touch.Contact{ID: 10, X: 175, Y: 205}, touch.Contact{ID: 11, X: 275, Y: 275}
			send(0, a)
			send(15, a, b)
			for i := 1; i <= 6; i++ {
				pa, pb := a, b
				pa.X += i * 15 * tc.dx
				pa.Y += i * 15 * tc.dy
				pb.X += (i - 1) * 15 * tc.dx
				pb.Y += (i - 1) * 15 * tc.dy
				send(40*i, pa, pb)
				pb.X += 15 * tc.dx
				pb.Y += 15 * tc.dy
				send(40*i+12, pa, pb)
			}
			if model.Page != tc.expected {
				t.Fatalf("failed to reach %d, got %d", tc.expected, model.Page)
			}
		})
	}
}
