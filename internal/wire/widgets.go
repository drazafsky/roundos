package wire

// MaxSubtitleBytes accommodates all 256 Unicode runes accepted by the shell
// editor, including four-byte characters. Legacy root hints mirror subtitles.
const MaxSubtitleBytes = 1024

// Widget is a shell-rendered, semantically themed control. Applications send
// values and actions; the shell owns positioning, hit-testing and focus.
// Widget IDs must remain stable across view updates to preserve focus.
type Widget struct {
	ID       string `json:"id"`
	Group    string `json:"group,omitempty"`   // negotiated semantic section identity
	Kind     string `json:"kind"`              // button, toggle, slider, label, progress, list_item, picker, time_picker, date_picker
	Variant  string `json:"variant,omitempty"` // primary, secondary (default), tertiary
	Label    string `json:"label"`
	Detail   string `json:"detail,omitempty"`
	Action   string `json:"action,omitempty"`
	Target   string `json:"target,omitempty"` // shell-owned navigation to another screen ID
	Value    int    `json:"value,omitempty"`
	Min      int    `json:"min,omitempty"`
	Max      int    `json:"max,omitempty"`
	Step     int    `json:"step,omitempty"`
	Checked  bool   `json:"checked,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
	Wrap     bool   `json:"wrap,omitempty"` // numeric/time picker values may wrap
}

// Dialog describes a modal overlay belonging to the foreground application.
// Actions and text are data; colors, hit regions and focus are shell-owned.
type Dialog struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Body          string `json:"body,omitempty"`
	Cancel        string `json:"cancel"`
	Confirm       string `json:"confirm"`
	CancelAction  string `json:"cancel_action"`
	ConfirmAction string `json:"confirm_action"`
	Destructive   bool   `json:"destructive,omitempty"`
}

// Screen describes a full-screen application view. The first screen is the
// root; Target links create a shell-owned, focus-restoring navigation stack.
type Screen struct {
	ID       string   `json:"id"`
	Layout   string   `json:"layout,omitempty"` // negotiated shell-owned template
	Title    string   `json:"title"`
	Hero     string   `json:"hero,omitempty"`
	Subtitle string   `json:"subtitle,omitempty"`
	Widgets  []Widget `json:"widgets,omitempty"`
}

// RootHeader retains the scalar presentation used by legacy shells and
// observers without duplicating it in application Go code.
func RootHeader(view View) View {
	if len(view.Screens) == 0 {
		return view
	}
	root := view.Screens[0]
	view.Title, view.Primary, view.Hint = root.Title, root.Hero, root.Subtitle
	for _, widget := range root.Widgets {
		if widget.Kind == "button" && widget.Variant == "primary" && widget.Action != "" {
			view.ButtonLabel, view.ButtonAction = widget.Label, widget.Action
			break
		}
	}
	return view
}

// CopyView prevents app-owned slices from altering the transport's diff
// baseline or the asynchronous shell renderer's immutable snapshots.
func CopyView(v View) View {
	if v.Dialog != nil {
		d := *v.Dialog
		v.Dialog = &d
	}
	v.Screens = append([]Screen(nil), v.Screens...)
	for i := range v.Screens {
		v.Screens[i].Widgets = append([]Widget(nil), v.Screens[i].Widgets...)
	}
	return v
}
