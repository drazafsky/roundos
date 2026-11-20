package shell

import (
	"roundos/internal/notify"
	"roundos/internal/wire"
	"testing"
	"time"
)

func TestNotificationCenterReturnAndDismiss(t *testing.T) {
	m := New()
	store := notify.New()
	m.Notifier = store
	m.Select(Timer)
	t0 := time.Unix(100, 0)
	r := store.Emit(wire.TimerID, notify.Request{Title: "Timer finished", Priority: notify.Actionable, Action: "open"}, t0, false)
	m.UpdateNotices(store.Items(), r, t0)
	if !m.OpenNotifications() || m.Page != NotificationCenter {
		t.Fatal("notification center")
	}
	if !m.OpenNotificationDetail() || m.Page != NotificationDetail {
		t.Fatal("detail")
	}
	if !m.Back() || m.Page != NotificationCenter {
		t.Fatal("back from detail")
	}
	m.OpenNotificationDetail()
	m.NotificationDetailFocus = 1
	if !m.DetailActivate() || m.Page != NotificationCenter || len(store.Items()) != 0 {
		t.Fatal("dismiss")
	}
	if !m.Back() || m.Page != Timer {
		t.Fatal("restore timer page")
	}
}
func TestNotificationSelectionPreservedOnIncoming(t *testing.T) {
	m := New()
	store := notify.New()
	m.Notifier = store
	at := time.Unix(100, 0)
	a := store.Emit(wire.TimerID, notify.Request{Title: "First", Priority: notify.Quiet}, at, false)
	m.UpdateNotices(store.Items(), a, at)
	old := m.Notices[0].ID
	b := store.Emit(wire.ClockID, notify.Request{Title: "Second", Priority: notify.Quiet}, at.Add(time.Second), false)
	m.UpdateNotices(store.Items(), b, at.Add(time.Second))
	if m.Notices[m.NotificationFocus].ID != old {
		t.Fatal("selection stolen by new notification")
	}
}
func TestSwitcherAndBannerExpiration(t *testing.T) {
	m := New()
	store := notify.New()
	at := time.Unix(100, 0)
	r := store.Emit(wire.TimerID, notify.Request{Title: "Done", Priority: notify.Actionable}, at, false)
	m.UpdateNotices(store.Items(), r, at)
	if m.Banner == nil || m.BannerExpired(at.Add(3*time.Second)) || !m.BannerExpired(at.Add(5*time.Second)) {
		t.Fatal("banner deadline")
	}
	m.Select(Clock)
	m.OpenSwitcher()
	if m.Page != Switcher {
		t.Fatal("switcher")
	}
	m.SwitcherFocus = 2
	if !m.SelectSwitcher() || m.Page != NotificationCenter {
		t.Fatal("rotary notification menu entry")
	}
	if !m.Back() || m.Page != Switcher {
		t.Fatal("return switcher")
	}
	if !m.Back() || m.Page != Clock {
		t.Fatal("return clock")
	}
}
