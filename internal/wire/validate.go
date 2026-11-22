package wire

import "fmt"

// ValidateView ensures a bounded, stable declarative tree before it crosses the
// process boundary. It does not permit application-supplied paint colors or
// touch rectangles: those are strictly shell-owned.
func ValidateView(v View) error {
	if v.Dialog != nil {
		d := v.Dialog
		if d.ID == "" || len(d.ID) > 64 || len(d.Title) > 80 || len(d.Body) > 240 || d.Cancel == "" || d.Confirm == "" || d.ConfirmAction == "" || d.CancelAction == "" {
			return fmt.Errorf("invalid modal dialog")
		}
	}
	if len(v.Screens) == 0 {
		return nil
	} // Compatibility with legacy 0.4 views.
	if len(v.Screens) > 16 {
		return fmt.Errorf("too many screens: %d", len(v.Screens))
	}
	seen := map[string]bool{}
	for _, s := range v.Screens {
		switch s.Layout {
		case "", "hero_list", "list", "form", "detail":
		default:
			return fmt.Errorf("unknown screen layout %q", s.Layout)
		}
		if s.ID == "" || len(s.ID) > 64 || seen[s.ID] {
			return fmt.Errorf("invalid or duplicate screen ID %q", s.ID)
		}
		seen[s.ID] = true
		if len(s.Widgets) > 64 {
			return fmt.Errorf("screen %q exceeds 64 widgets", s.ID)
		}
		ids := map[string]bool{}
		for _, w := range s.Widgets {
			if len(w.Group) > 64 || w.Group != "" && s.Layout == "" {
				return fmt.Errorf("invalid widget section")
			}
			if w.ID == "" || len(w.ID) > 64 || ids[w.ID] {
				return fmt.Errorf("invalid or duplicate widget ID %q", w.ID)
			}
			ids[w.ID] = true
			switch w.Kind {
			case "button", "toggle", "slider", "label", "progress", "list_item", "picker", "time_picker", "date_picker":
			default:
				return fmt.Errorf("unknown widget kind %q", w.Kind)
			}
			if w.Variant != "" && w.Variant != "primary" && w.Variant != "secondary" && w.Variant != "tertiary" {
				return fmt.Errorf("unknown variant %q", w.Variant)
			}
			if (w.Kind == "picker" || w.Kind == "time_picker" || w.Kind == "date_picker") && w.Step < 0 {
				return fmt.Errorf("negative picker step for %q", w.ID)
			}
			if (w.Kind == "slider" || w.Kind == "progress" || w.Kind == "picker" || w.Kind == "time_picker" || w.Kind == "date_picker") && (w.Max <= w.Min || w.Value < w.Min || w.Value > w.Max) {
				return fmt.Errorf("invalid range for %q", w.ID)
			}
		}
	}
	for _, s := range v.Screens {
		for _, w := range s.Widgets {
			if w.Target != "" && !seen[w.Target] {
				return fmt.Errorf("unknown screen target %q", w.Target)
			}
		}
	}
	return nil
}
