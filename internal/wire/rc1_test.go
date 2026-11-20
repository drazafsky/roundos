package wire

import "testing"

func TestRC1DialogValidationAndSnapshot(t *testing.T) {
	v := View{Screens: []Screen{{ID: "root", Widgets: []Widget{{ID: "pick", Kind: "time_picker", Min: 0, Max: 1439, Value: 480, Step: 5}, {ID: "list", Kind: "list_item", Label: "NAME", Action: "open"}}}}, Dialog: &Dialog{ID: "delete", Title: "DELETE", Body: "Are you sure?", Cancel: "Cancel", Confirm: "Delete", CancelAction: "cancel", ConfirmAction: "confirm", Destructive: true}}
	if err := ValidateView(v); err != nil {
		t.Fatal(err)
	}
	copy := CopyView(v)
	copy.Dialog.Title = "CHANGED"
	if v.Dialog.Title == "CHANGED" {
		t.Fatal("dialog not copied")
	}
	v.Dialog.CancelAction = ""
	if err := ValidateView(v); err == nil {
		t.Fatal("invalid dialog accepted")
	}
	v.Dialog = nil
	v.Screens[0].Widgets[0].Value = 2000
	if err := ValidateView(v); err == nil {
		t.Fatal("out of range time picker accepted")
	}
}
