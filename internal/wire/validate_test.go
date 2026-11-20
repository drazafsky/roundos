package wire

import "testing"

func TestValidateWidgetTree(t *testing.T) {
	good := View{Screens: []Screen{{ID: "root", Widgets: []Widget{{ID: "a", Kind: "button", Target: "second", Variant: "primary"}}}, {ID: "second", Widgets: []Widget{{ID: "b", Kind: "slider", Value: 5, Min: 0, Max: 10, Step: 1}}}}}
	if err := ValidateView(good); err != nil {
		t.Fatalf("valid tree rejected: %v", err)
	}
	bad := CopyView(good)
	bad.Screens[0].Widgets[0].Target = "unknown"
	if ValidateView(bad) == nil {
		t.Fatal("unknown screen allowed")
	}
	bad = CopyView(good)
	bad.Screens[0].Widgets = append(bad.Screens[0].Widgets, bad.Screens[0].Widgets[0])
	if ValidateView(bad) == nil {
		t.Fatal("duplicate ID allowed")
	}
	bad = CopyView(good)
	bad.Screens[1].Widgets[0].Value = 11
	if ValidateView(bad) == nil {
		t.Fatal("invalid slider value allowed")
	}
	bad = CopyView(good)
	bad.Screens[1].Widgets[0].Variant = "customPink"
	if ValidateView(bad) == nil {
		t.Fatal("nonsemantic app style allowed")
	}
}
