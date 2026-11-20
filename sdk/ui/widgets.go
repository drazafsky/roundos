// Package ui contains the declarative RoundOS control constructors. UI colors
// and geometry deliberately never appear in application source.
package ui

import "roundos/internal/wire"

// Public aliases let standalone Go modules consume the SDK without directly
// importing RoundOS's Go-internal wire package. Do not mutate views after
// Publish; each new view is a declarative snapshot.
type View = wire.View
type ScreenSpec = wire.Screen
type Widget = wire.Widget
type Dialog = wire.Dialog

func Button(id, label, action string) wire.Widget {
	return wire.Widget{ID: id, Kind: "button", Label: label, Action: action}
}

// PrimaryButton uses the system accent, never an application color.
func PrimaryButton(id, label, action string) wire.Widget {
	w := Button(id, label, action)
	w.Variant = "primary"
	return w
}
func Link(id, label, screenID string) wire.Widget {
	return wire.Widget{ID: id, Kind: "button", Label: label, Target: screenID}
}
func Toggle(id, label, action string, checked bool) wire.Widget {
	return wire.Widget{ID: id, Kind: "toggle", Label: label, Action: action, Checked: checked}
}
func Slider(id, label, action string, value, minValue, maxValue, step int) wire.Widget {
	return wire.Widget{ID: id, Kind: "slider", Label: label, Action: action, Value: value, Min: minValue, Max: maxValue, Step: step}
}
func ListItem(id, label, detail, action string) wire.Widget {
	return wire.Widget{ID: id, Kind: "list_item", Label: label, Detail: detail, Action: action}
}
func NumberPicker(id, label, action string, value, minValue, maxValue, step int) wire.Widget {
	return wire.Widget{ID: id, Kind: "picker", Label: label, Action: action, Value: value, Min: minValue, Max: maxValue, Step: step}
}
func TimePicker(id, label, action string, minuteOfDay, step int) wire.Widget {
	return wire.Widget{ID: id, Kind: "time_picker", Label: label, Action: action, Value: minuteOfDay, Min: 0, Max: 1439, Step: step, Wrap: true}
}

// DatePickerDays uses UTC days since Unix epoch, avoiding timezone shifts
// while users edit calendar dates (the application owns date semantics).
func DatePickerDays(id, label, action string, epochDay, minDay, maxDay int) wire.Widget {
	return wire.Widget{ID: id, Kind: "date_picker", Label: label, Action: action, Value: epochDay, Min: minDay, Max: maxDay, Step: 1}
}
func ConfirmDialog(id, title, body, confirmAction, cancelAction string, destructive bool) *wire.Dialog {
	return &wire.Dialog{ID: id, Title: title, Body: body, Cancel: "CANCEL", Confirm: "CONFIRM", CancelAction: cancelAction, ConfirmAction: confirmAction, Destructive: destructive}
}
func Label(id, label string) wire.Widget {
	return wire.Widget{ID: id, Kind: "label", Label: label}
}
func Progress(id, label string, value, minValue, maxValue int) wire.Widget {
	return wire.Widget{ID: id, Kind: "progress", Label: label, Value: value, Min: minValue, Max: maxValue}
}
func Screen(id, title, hero, subtitle string, items ...wire.Widget) wire.Screen {
	return wire.Screen{ID: id, Title: title, Hero: hero, Subtitle: subtitle, Widgets: items}
}
