// Package systemgestures maps recognized touch gestures to user-configurable
// shell actions. Gesture recognition remains independent of DRM rendering.
package systemgestures

import (
	"encoding/json"
	"math"
	"os"

	"roundos/internal/gesture"
)

type Config struct {
	LeftBack         bool `json:"left_back"`
	TopNotifications bool `json:"top_notifications"`
	MaxContacts      int  `json:"max_contacts"` // verified hardware profile, not a decoder cap
	TwoSwitch        bool `json:"two_finger_switch"`
	TwoLauncher      bool `json:"two_finger_launcher"`
	// The current HyperPixel evdev path yields two-finger vertical swipe labels
	// opposite to the direction observed by the user. This is deliberately a
	// system-shortcut mapping ONLY: do not mirror tap/drag coordinates or
	// one-finger edge gestures to compensate for two-finger launcher direction.
	TwoVerticalReverse bool   `json:"two_finger_vertical_reverse"`
	ThreeSwitch        bool   `json:"three_finger_switch"`
	ThreeLauncher      bool   `json:"three_finger_launcher"`
	FourSwitcher       bool   `json:"four_finger_switcher"`
	FiveApp            bool   `json:"five_finger_app"`
	EdgeWidth          int    `json:"edge_width_px"`
	Sensitivity        string `json:"sensitivity"` // low, balanced, high
}

func Default() Config {
	return Config{LeftBack: true, TopNotifications: true, MaxContacts: 2, TwoVerticalReverse: true, EdgeWidth: 32, Sensitivity: "balanced"}
}
func (c Config) Valid() bool {
	return c.MaxContacts >= 1 && c.MaxContacts <= 16 && c.EdgeWidth >= 20 && c.EdgeWidth <= 48 && (c.Sensitivity == "low" || c.Sensitivity == "balanced" || c.Sensitivity == "high")
}
func Load(path string) (Config, error) {
	c := Default()
	if path == "" {
		return c, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	if !c.Valid() {
		return c, &configError{}
	}
	return c, nil
}

type configError struct{}

func (*configError) Error() string {
	return "invalid gestures configuration (max_contacts 1..16; edge 20..48px; sensitivity low/balanced/high)"
}

type Action string

const (
	None          Action = ""
	Back          Action = "back"
	Notifications Action = "notifications"
	NextApp       Action = "next_app"
	PreviousApp   Action = "previous_app"
	Launcher      Action = "launcher"
	Switcher      Action = "switcher"
)

type Router struct {
	Config   Config
	tracking bool
	x, y     float64
	claimed  bool
}

func New(c Config) *Router { return &Router{Config: c} }
func (r *Router) Reset()   { r.tracking = false; r.claimed = false }
func (r *Router) threshold() float64 {
	switch r.Config.Sensitivity {
	case "high":
		return 45
	case "low":
		return 68
	default:
		return 55
	}
}

func (r *Router) Handle(ev gesture.Event) Action {
	switch ev.Type {
	case gesture.Down:
		r.x, r.y = ev.X, ev.Y
		r.tracking = true
		r.claimed = false
	case gesture.Cancel, gesture.Up:
		r.Reset()
	case gesture.Drag, gesture.Swipe:
		if !r.tracking || r.claimed || ev.Fingers != 1 {
			return None
		}
		dx, dy := ev.DX, ev.DY
		threshold := r.threshold()
		// A circular edge varies with position. A rectangular 32px strip
		// would miss valid touches near the upper and lower perimeter.
		if r.Config.LeftBack && r.x <= leftEdge(r.y)+float64(r.Config.EdgeWidth) && dx >= threshold && dx >= 1.55*math.Abs(dy) {
			r.claimed = true
			return Back
		}
		if r.Config.TopNotifications && r.y <= topEdge(r.x)+float64(r.Config.EdgeWidth) && dy >= threshold && dy >= 1.55*math.Abs(dx) {
			r.claimed = true
			return Notifications
		}
	case gesture.TwoSwipe:
		if ev.Fingers != 2 || r.Config.MaxContacts < 2 {
			return None
		}
		switch ev.Direction {
		case "left":
			if r.Config.TwoSwitch {
				return NextApp
			}
		case "right":
			if r.Config.TwoSwitch {
				return PreviousApp
			}
		case "up":
			if r.Config.TwoLauncher && !r.Config.TwoVerticalReverse {
				return Launcher
			}
		case "down":
			if r.Config.TwoLauncher && r.Config.TwoVerticalReverse {
				return Launcher
			}
		}
	case gesture.ThreeSwipe:
		if r.Config.MaxContacts < 3 || ev.Fingers != 3 {
			return None
		}
		switch ev.Direction {
		case "left":
			if r.Config.ThreeSwitch {
				return NextApp
			}
		case "right":
			if r.Config.ThreeSwitch {
				return PreviousApp
			}
		case "up":
			if r.Config.ThreeLauncher {
				return Launcher
			}
		}
	case gesture.FourTap:
		if r.Config.MaxContacts >= 4 && ev.Fingers == 4 && r.Config.FourSwitcher {
			return Switcher
		}
	}
	return None
}
func leftEdge(y float64) float64 {
	dy := y - 240
	return 240 - math.Sqrt(math.Max(0, 240*240-dy*dy))
}
func topEdge(x float64) float64 {
	dx := x - 240
	return 240 - math.Sqrt(math.Max(0, 240*240-dx*dx))
}
