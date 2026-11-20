package wire

// ViewPatch transmits only changed scalar properties of an already published
// app view. Nil means "unchanged"; a non-nil empty string clears the field.
// This intentionally remains a small typed protocol, not a generic widget tree.
type ViewPatch struct {
	Title        *string `json:"title,omitempty"`
	Primary      *string `json:"primary,omitempty"`
	Secondary    *string `json:"secondary,omitempty"`
	Hint         *string `json:"hint,omitempty"`
	ButtonLabel  *string `json:"button_label,omitempty"`
	ButtonAction *string `json:"button_action,omitempty"`
	RotaryAction *string `json:"rotary_action,omitempty"`
}

func DiffView(before, after View) (ViewPatch, int) {
	var p ViewPatch
	changed := 0
	if before.Title != after.Title {
		s := after.Title
		p.Title = &s
		changed++
	}
	if before.Primary != after.Primary {
		s := after.Primary
		p.Primary = &s
		changed++
	}
	if before.Secondary != after.Secondary {
		s := after.Secondary
		p.Secondary = &s
		changed++
	}
	if before.Hint != after.Hint {
		s := after.Hint
		p.Hint = &s
		changed++
	}
	if before.ButtonLabel != after.ButtonLabel {
		s := after.ButtonLabel
		p.ButtonLabel = &s
		changed++
	}
	if before.ButtonAction != after.ButtonAction {
		s := after.ButtonAction
		p.ButtonAction = &s
		changed++
	}
	if before.RotaryAction != after.RotaryAction {
		s := after.RotaryAction
		p.RotaryAction = &s
		changed++
	}
	return p, changed
}

func (p ViewPatch) Apply(v View) View {
	if p.Title != nil {
		v.Title = *p.Title
	}
	if p.Primary != nil {
		v.Primary = *p.Primary
	}
	if p.Secondary != nil {
		v.Secondary = *p.Secondary
	}
	if p.Hint != nil {
		v.Hint = *p.Hint
	}
	if p.ButtonLabel != nil {
		v.ButtonLabel = *p.ButtonLabel
	}
	if p.ButtonAction != nil {
		v.ButtonAction = *p.ButtonAction
	}
	if p.RotaryAction != nil {
		v.RotaryAction = *p.RotaryAction
	}
	return v
}
