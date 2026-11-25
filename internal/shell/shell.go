package shell

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"reflect"
	"strings"
	"time"

	"roundos/internal/appearance"
	"roundos/internal/notify"
	"roundos/internal/textedit"
	"roundos/internal/touch"
	"roundos/internal/update"
	"roundos/internal/wifi"
	"roundos/internal/wire"
)

const Width = 480
const Height = 480
const (
	Home = iota
	Clock
	Timer
	Diagnostics
)

type Model struct {
	Page                    int
	Selected                int
	AppCards                []AppCard
	carouselFrom            float64
	carouselTarget          float64
	carouselStarted         time.Time
	carouselNextFrame       time.Time
	AppPages                map[string]int
	PageApps                map[int]string
	WiFi                    WiFiState
	Updates                 UpdateState
	Views                   map[int]wire.View
	ViewLayouts             map[int]uint64
	widgetInput             *wire.UIEvent
	Status                  map[int]string
	LastTouch               image.Point
	TouchUntil              time.Time
	Contacts                []touch.Contact
	InputRevision           uint64 // changes when an active view changes touch targets
	LastGesture             string
	SupportedContacts       int // configured/verified hardware profile; not the decoder limit
	Notices                 []notify.Item
	NotificationFocus       int
	NotificationDetailFocus int
	Notifier                *notify.Store
	SwitcherFocus           int
	ShellParents            []int
	Banner                  *notify.Item
	BannerUntil             time.Time
	Critical                *notify.Item
	Appearance              appearance.Settings
	AppearanceDirty         bool
	AccessFocus             int
	AccessParent            int
	Editor                  textedit.State
	EditorParent            int
	EditorAppID             string
	EditorLabel             string
	EditorRequestID         string
	PendingEditorResult     *wire.TextResult
	PendingEditorApp        string
	EditorFocus             int
	DiscardFocus            int
	AppStacks               map[int][]NavFrame
	EditingWidget           string
	WidgetEditValue         int
	WidgetDelta             int
	TouchSlider             string
	DragOriginScroll        int
	DragOriginY             int
	DragOriginPx            float64
	DragLastDY              float64
	DragVelocity            float64
	DragLastAt              time.Time
	KineticVelocity         float64
	KineticLast             time.Time
	KineticUntil            time.Time
	KineticPage             int
	DialogID                string
	DialogFocus             int
	DialogHidden            bool
	PendingDialogAction     string
	PendingDialogApp        string
	PickerID                string
	PickerValue             int
	PickerAction            string
}

// Snapshot creates an immutable copy for the asynchronous renderer. The shell
// remains the sole writer; drawing never reads live mutable maps or contacts.
func (m *Model) Snapshot() *Model {
	copyOf := *m
	copyOf.AppCards = append([]AppCard(nil), m.AppCards...)
	copyOf.AppPages = make(map[string]int, len(m.AppPages))
	for k, v := range m.AppPages {
		copyOf.AppPages[k] = v
	}
	copyOf.PageApps = make(map[int]string, len(m.PageApps))
	for k, v := range m.PageApps {
		copyOf.PageApps[k] = v
	}
	copyOf.Views = make(map[int]wire.View, len(m.Views))
	for k, v := range m.Views {
		copyOf.Views[k] = wire.CopyView(v)
	}
	copyOf.ViewLayouts = make(map[int]uint64, len(m.ViewLayouts))
	for page, revision := range m.ViewLayouts {
		copyOf.ViewLayouts[page] = revision
	}
	copyOf.widgetInput = nil
	copyOf.Status = make(map[int]string, len(m.Status))
	for k, v := range m.Status {
		copyOf.Status[k] = v
	}
	copyOf.Contacts = append([]touch.Contact(nil), m.Contacts...)
	copyOf.Notices = append([]notify.Item(nil), m.Notices...)
	copyOf.WiFi.Networks = append([]wifi.Network(nil), m.WiFi.Networks...)
	copyOf.WiFi.Pending = nil
	copyOf.Updates.Pending = nil
	copyOf.Updates.PendingInstall = nil
	copyOf.Updates.PendingRelease = copyUpdateRelease(m.Updates.PendingRelease)
	copyOf.Updates.Notice = copyUpdateRelease(m.Updates.Notice)
	if m.Updates.Status.Result.Release != nil {
		r := *m.Updates.Status.Result.Release
		r.Assets = append([]update.Asset(nil), r.Assets...)
		copyOf.Updates.Status.Result.Release = &r
	}
	copyOf.AppStacks = make(map[int][]NavFrame, len(m.AppStacks))
	for k, v := range m.AppStacks {
		copyOf.AppStacks[k] = append([]NavFrame(nil), v...)
	}
	copyOf.ShellParents = append([]int(nil), m.ShellParents...)
	if m.Banner != nil {
		b := *m.Banner
		copyOf.Banner = &b
	}
	if m.Critical != nil {
		b := *m.Critical
		copyOf.Critical = &b
	}
	return &copyOf
}

func New() *Model {
	m := &Model{Page: Home, Appearance: appearance.Default(), SupportedContacts: 2, Views: map[int]wire.View{}, AppStacks: map[int][]NavFrame{}, Status: map[int]string{Clock: "WAITING FOR APP", Timer: "WAITING FOR APP"}}
	m.SetInstalledApps(nil)
	m.Updates.Status.Preferences = update.DefaultPreferences()
	return m
}
func AppID(page int) string {
	switch page {
	case Clock:
		return wire.ClockID
	case Timer:
		return wire.TimerID
	}
	return ""
}
func PageForApp(id string) int {
	switch id {
	case wire.ClockID:
		return Clock
	case wire.TimerID:
		return Timer
	}
	return Home
}
func (m *Model) Select(page int) bool {
	if page < Home || (page > ReleaseNotes && !m.IsAppPage(page)) || page == m.Page {
		return false
	}
	if (m.Page == TextEditor || m.Page == DiscardPrompt) && page != TextEditor && page != DiscardPrompt {
		// A system shortcut can close the Wi-Fi password editor without
		// invoking finishEditor. Clear its pending purpose as well.
		m.WiFi.EditorPurpose = ""
		if m.Editor.Secure {
			m.Editor.ClearSensitive()
		}
		if m.EditorAppID != "" {
			if m.PendingEditorResult == nil {
				m.PendingEditorResult = &wire.TextResult{ID: m.EditorRequestID, Canceled: true}
				m.PendingEditorApp = m.EditorAppID
				m.Editor.Cancel()
			}
			// The result already owns its routing IDs. A saved shell return
			// path must not resurrect this completed app editor request.
			m.EditorAppID, m.EditorRequestID = "", ""
			for i, parent := range m.ShellParents {
				if parent == TextEditor || parent == DiscardPrompt {
					m.ShellParents[i] = m.EditorParent
				}
			}
			if m.AccessParent == TextEditor || m.AccessParent == DiscardPrompt {
				m.AccessParent = m.EditorParent
			}
		}
	}
	if page != m.Page {
		m.carouselStarted = time.Time{}
		m.cancelPicker()
		m.StopKinetic()
		m.cancelWidgetEdit()
	}
	m.Page = page
	if page == Home || m.IsAppPage(page) || page == Diagnostics {
		m.ShellParents = nil
	}
	return true
}
func (m *Model) SetView(id string, v wire.View) bool {
	return m.SetViewRevision(id, v, 0)
}

func (m *Model) SetViewRevision(id string, v wire.View, layout uint64) bool {
	if wire.ValidateView(v) != nil {
		return false
	}
	page := m.PageForApp(id)
	if page == Home {
		return false
	}
	if m.ViewLayouts == nil {
		m.ViewLayouts = map[int]uint64{}
	}
	m.ViewLayouts[page] = layout
	old, ok := m.Views[page]
	status := m.Status[page]
	if ok && reflect.DeepEqual(old, v) && status == "" {
		return false
	}
	beforeScreen, hadScreen := m.CurrentScreen(page)
	m.reconcileReplacement(page, old, v)
	m.Views[page] = wire.CopyView(v)
	m.syncNavigation(page)
	if m.Page == page {
		afterScreen, hasScreen := m.CurrentScreen(page)
		if hadScreen != hasScreen || beforeScreen.ID != afterScreen.ID {
			// Removing a parent can drop an otherwise unchanged child from
			// the navigation stack. Its input and edits belong to that child.
			m.cancelReloadContact()
			m.cancelPicker()
			m.cancelWidgetEdit()
		}
		m.syncDialog(page)
	}
	m.Status[page] = ""
	return m.Page == page
}
func (m *Model) SetStatus(id, status string) bool {
	page := m.PageForApp(id)
	if page == Home {
		return false
	}
	if m.Status[page] == status {
		return false
	}
	m.Status[page] = status
	return m.Page == page
}
func (m *Model) UpdateContacts(contacts []touch.Contact) bool {
	if m.Page != Diagnostics {
		return false
	}
	m.Contacts = append(m.Contacts[:0], contacts...)
	return true
}
func (m *Model) SetGesture(label string) bool {
	if m.Page != Diagnostics {
		return false
	}
	m.LastGesture = strings.ToUpper(strings.ReplaceAll(label, "_", " "))
	return true
}
func (m *Model) TwoFingerSwipe(direction string) bool {
	if m.Page == Diagnostics {
		return false
	}
	switch direction {
	case "up":
		return m.Select(Home)
	case "left":
		if m.Page == Clock || m.Page == Home {
			return m.Select(Timer)
		}
	case "right":
		if m.Page == Timer || m.Page == Home {
			return m.Select(Clock)
		}
	}
	return false
}
func (m *Model) Touch(x, y int) {
	m.LastTouch = image.Pt(x, y)
	m.TouchUntil = time.Now().Add(180 * time.Millisecond)
}

// Tap returns whether a repaint is needed and an app-owned action to dispatch.
func (m *Model) Tap(x, y int) (bool, string) {
	if !within(x, y, 230) {
		return false, ""
	}
	m.Touch(x, y)
	if m.PickerID != "" {
		return m.pickerTap(x, y)
	}
	if m.hasVisibleDialog() {
		return m.dialogTap(x, y)
	}
	if m.Page == Settings || m.Page == WiFiPage || m.Page == WiFiConfirm {
		return m.SettingsTap(x, y), ""
	}
	if m.Page == UpdatesPage {
		return m.UpdatesTap(x, y), ""
	}
	if m.Page == UpdatePrompt {
		return m.UpdatePromptTap(x, y), ""
	}
	if m.Page == ReleaseNotes {
		return m.releaseNotesTap(x, y), ""
	}
	if m.Page == TextEditor || m.Page == Accessibility || m.Page == DiscardPrompt {
		return m.TapSupplement(x, y)
	}
	if m.Page == NotificationCenter {
		if y >= 365 {
			return m.Back(), ""
		}
		if y >= 147 && y <= 358 {
			start := max(0, m.NotificationFocus-2)
			index := start + (y-147)/75
			if index >= 0 && index < len(m.Notices) {
				m.NotificationFocus = index
				return m.OpenNotificationDetail(), ""
			}
		}
		return false, ""
	}
	if m.Page == NotificationDetail {
		if y >= 275 && y <= 423 {
			m.NotificationDetailFocus = min(2, max(0, (y-275)/51))
			return m.DetailActivate(), ""
		}
		return false, ""
	}
	if m.Page == Switcher {
		if y >= 365 {
			return m.Back(), ""
		}
		if y >= 133 && y <= 355 {
			start := max(0, m.SwitcherFocus-2)
			index := start + (y-133)/70
			if index < len(m.SwitcherEntries()) {
				m.SwitcherFocus = index
				return m.SelectSwitcher(), ""
			}
		}
		return false, ""
	}
	if m.hasWidgets(m.Page) {
		return m.WidgetTap(x, y)
	}
	if m.Page == Home {
		if y > 165 && y < 340 {
			if x < 123 {
				return m.moveCarousel(-1, time.Now()), ""
			} else if x > 357 {
				return m.moveCarousel(1, time.Now()), ""
			} else {
				if m.carouselAnimating(time.Now()) {
					return false, ""
				}
				m.Press()
			}
		}
		return true, ""
	}
	if y >= 355 {
		m.Select(Home)
		return true, ""
	}
	if m.Page == Diagnostics {
		return true, ""
	}
	if x >= 153 && x <= 327 && y >= 249 && y <= 307 {
		return true, m.Views[m.Page].ButtonAction
	}
	return true, ""
}
func (m *Model) Rotate(delta int) (bool, string, int) {
	if delta == 0 {
		return false, "", 0
	}
	if m.PickerID != "" {
		return m.pickerRotate(delta), "", 0
	}
	if m.hasVisibleDialog() {
		return m.dialogRotate(delta), "", 0
	}
	if m.Page == Settings || m.Page == WiFiPage || m.Page == WiFiConfirm {
		return m.SettingsRotate(delta), "", 0
	}
	if m.Page == UpdatesPage {
		return m.UpdatesRotate(delta), "", 0
	}
	if m.Page == UpdatePrompt {
		return m.UpdatePromptRotate(delta), "", 0
	}
	if m.Page == ReleaseNotes {
		return m.ReleaseNotesMove(delta), "", 0
	}
	if m.Page == TextEditor || m.Page == Accessibility || m.Page == DiscardPrompt {
		return m.RotateSupplement(delta)
	}
	if m.Page == NotificationCenter {
		if len(m.Notices) == 0 {
			return false, "", 0
		}
		next := max(0, min(len(m.Notices)-1, m.NotificationFocus+delta))
		if next == m.NotificationFocus {
			return false, "", 0
		}
		m.NotificationFocus = next
		return true, "", 0
	}
	if m.Page == NotificationDetail {
		next := max(0, min(2, m.NotificationDetailFocus+delta))
		if next == m.NotificationDetailFocus {
			return false, "", 0
		}
		m.NotificationDetailFocus = next
		return true, "", 0
	}
	if m.Page == Switcher {
		next := max(0, min(len(m.SwitcherEntries())-1, m.SwitcherFocus+delta))
		if next == m.SwitcherFocus {
			return false, "", 0
		}
		m.SwitcherFocus = next
		return true, "", 0
	}
	if m.Page == Home {
		return m.moveCarousel(delta, time.Now()), "", 0
	}
	if m.hasWidgets(m.Page) {
		return m.WidgetRotate(delta), "", 0
	}
	if a := m.Views[m.Page].RotaryAction; a != "" {
		return false, a, delta
	}
	return false, "", 0
}

// Activate processes a short push-button press. The dedicated system long
// press returns Home; a short press activates the foreground app's button.
func (m *Model) Activate() (bool, string) {
	if m.Critical != nil {
		return m.AcknowledgeCritical(), ""
	}
	if m.PickerID != "" {
		return m.pickerCommit()
	}
	if m.hasVisibleDialog() {
		return m.dialogActivate()
	}
	if m.Page == Settings || m.Page == WiFiPage || m.Page == WiFiConfirm {
		return m.SettingsActivate(), ""
	}
	if m.Page == UpdatesPage {
		return m.UpdatesActivate(time.Now()), ""
	}
	if m.Page == UpdatePrompt {
		return m.UpdatePromptActivate(), ""
	}
	if m.Page == ReleaseNotes {
		return m.Select(UpdatePrompt), ""
	}
	if m.Page == TextEditor || m.Page == Accessibility || m.Page == DiscardPrompt {
		return m.ActivateSupplement()
	}
	if m.Page == NotificationCenter {
		return m.OpenNotificationDetail(), ""
	}
	if m.Page == NotificationDetail {
		return m.DetailActivate(), ""
	}
	if m.Page == Switcher {
		return m.SelectSwitcher(), ""
	}
	if m.Page == Home {
		return m.Press(), ""
	}
	if m.Page == Diagnostics {
		return m.Select(Home), ""
	}
	if m.hasWidgets(m.Page) {
		return m.ActivateWidget()
	}
	if action := m.Views[m.Page].ButtonAction; action != "" {
		return false, action
	}
	return m.Select(Home), ""
}

func (m *Model) Press() bool {
	if m.Page == Home {
		return m.SelectCarousel(m.Selected)
	}
	return m.Select(Home)
}

// Draw is intentionally stateless: input/state transitions trigger frames, not a frame tick.
func Draw(m *Model, now time.Time) *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, Width, Height))
	DrawInto(im, m, now)
	return im
}

// DrawInto reuses a fixed-size framebuffer to avoid allocating 900 KiB on every
// touch or timer repaint. It is called by the single-threaded shell UI loop.
func DrawInto(im *image.RGBA, m *Model, now time.Time) {
	if im.Bounds() != image.Rect(0, 0, Width, Height) || im.Stride != Width*4 {
		panic("RoundOS: DrawInto requires contiguous 480x480 RGBA")
	}
	clear(im.Pix)
	pal := m.Appearance.Palette()
	bg, panel, accent, white, grey := pal.Background, pal.Surface, pal.Accent, pal.PrimaryText, pal.SecondaryText
	disc(im, 240, 240, 239, bg)
	ring(im, 240, 240, 235, 2, panel)
	switch m.Page {
	case TextEditor, Accessibility, DiscardPrompt:
		m.DrawSupplement(im)
	case Settings, WiFiPage, WiFiConfirm:
		m.DrawSettings(im)
	case UpdatesPage:
		m.DrawUpdates(im, now)
	case UpdatePrompt:
		m.DrawUpdatePrompt(im)
	case ReleaseNotes:
		m.DrawReleaseNotes(im)
	case Home:
		drawCarousel(im, m, now)
	case Diagnostics:
		textTheme(im, 240, 66, 3, "TOUCH TEST", accent, true, m.Appearance)
		textTheme(im, 240, 103, 2, fmt.Sprintf("TOUCH %d OF %d", len(m.Contacts), m.SupportedContacts), white, true, m.Appearance)
		if m.LastGesture != "" {
			textTheme(im, 240, 132, 2, m.LastGesture, grey, true, m.Appearance)
		}
		for _, c := range m.Contacts {
			ring(im, c.X, c.Y, 25, 4, accent)
			ring(im, c.X, c.Y, 9, 2, white)
			textTheme(im, c.X, c.Y+32, 2, fmt.Sprintf("%d", c.ID), white, true, m.Appearance)
		}
		button(im, 240, 398, 140, 42, "HOME", panel, white)
	case NotificationCenter:
		textTheme(im, 240, 92, 3, "NOTIFICATIONS", accent, true, m.Appearance)
		if len(m.Notices) == 0 {
			textTheme(im, 240, 226, 2, "NO NOTIFICATIONS", grey, true, m.Appearance)
		}
		start := 0
		if m.NotificationFocus >= 3 {
			start = m.NotificationFocus - 2
		}
		for i := start; i < len(m.Notices) && i < start+3; i++ {
			y := 171 + (i-start)*75
			rect(im, 94, y-26, 386, y+37, panel)
			if i == m.NotificationFocus {
				outlineRect(im, 92, y-28, 388, y+39, accent)
			}
			title := strings.ToUpper(m.Notices[i].Request.Title)
			if len(title) > 18 {
				title = title[:18]
			}
			textTheme(im, 240, y-8, 2, title, white, true, m.Appearance)
			textTheme(im, 240, y+17, 1, strings.ToUpper(string(m.Notices[i].Request.Priority)), grey, true, m.Appearance)
		}
		button(im, 240, 399, 140, 42, "BACK", panel, white)
	case NotificationDetail:
		textTheme(im, 240, 91, 3, "NOTIFICATION", accent, true, m.Appearance)
		item, ok := m.SelectedNotification()
		if ok {
			title := strings.ToUpper(item.Request.Title)
			if len(title) > 18 {
				title = title[:18]
			}
			textTheme(im, 240, 176, 2, title, white, true, m.Appearance)
			body := strings.ToUpper(item.Request.Body)
			if len(body) > 22 {
				body = body[:22]
			}
			textTheme(im, 240, 217, 1, body, grey, true, m.Appearance)
			source := strings.ToUpper(item.App)
			if len(source) > 21 {
				source = source[:21]
			}
			textTheme(im, 240, 246, 1, source, grey, true, m.Appearance)
		}
		for i, name := range []string{"OPEN APP", "DISMISS", "BACK"} {
			y := 299 + i*51
			rect(im, 142, y-18, 338, y+22, panel)
			if m.NotificationDetailFocus == i {
				outlineRect(im, 140, y-20, 340, y+24, accent)
			}
			textTheme(im, 240, y-8, 2, name, white, true, m.Appearance)
		}
	case Switcher:
		textTheme(im, 240, 91, 3, "RUNNING APPS", accent, true, m.Appearance)
		items := m.SwitcherEntries()
		foreground := Home
		if len(m.ShellParents) > 0 {
			foreground = m.ShellParents[len(m.ShellParents)-1]
		}
		start := max(0, m.SwitcherFocus-2)
		for i := start; i < len(items) && i < start+3; i++ {
			y := 155 + (i-start)*70
			rect(im, 106, y-19, 374, y+41, panel)
			if m.SwitcherFocus == i {
				outlineRect(im, 104, y-21, 376, y+43, accent)
			}
			appIndex := -1
			switch i {
			case 0, 1:
				appIndex = i
			case 6:
				appIndex = 2
			}
			if appIndex >= 0 {
				foreground := grey
				if m.SwitcherFocus == i {
					foreground = accent
				}
				drawAppIcon(im, m.AppCards[appIndex], 132, y+7, 15, foreground)
			}
			textTheme(im, 240, y-8, 2, items[i], white, true, m.Appearance)
			status := "SHELL ACTION"
			if i == 0 || i == 1 || i >= 7 {
				page := Clock
				if i == 1 {
					page = Timer
				}
				if i >= 7 && i-7+3 < len(m.AppCards) {
					page = m.AppCards[i-7+3].Page
				}
				status = "BACKGROUND"
				if page == foreground {
					status = "FOREGROUND"
				}
				if m.Status[page] != "" {
					status = "OFFLINE"
				}
			} else if i == 2 {
				status = fmt.Sprintf("%d ALERTS", len(m.Notices))
			}
			textTheme(im, 240, y+17, 1, status, grey, true, m.Appearance)
		}
		button(im, 240, 423, 140, 34, "BACK", panel, white)
	default:
		if !m.IsAppPage(m.Page) {
			break
		}
		if m.hasWidgets(m.Page) {
			drawWidgetScreen(im, m)
			break
		}
		v := m.Views[m.Page]
		status := m.Status[m.Page]
		if status != "" {
			textTheme(im, 240, 90, 3, "ROUNDOS", accent, true, m.Appearance)
			textTheme(im, 240, 218, 2, status, grey, true, m.Appearance)
		} else {
			title := strings.ToUpper(v.Title)
			if title == "" {
				title = "APPLICATION"
			}
			textTheme(im, 240, 80, 3, title, accent, true, m.Appearance)
			size := 6
			if len(v.Primary) > 7 {
				size = 4
			}
			textTheme(im, 240, 208, size, strings.ToUpper(v.Primary), white, true, m.Appearance)
			if v.ButtonLabel != "" {
				button(im, 240, 278, 174, 58, strings.ToUpper(v.ButtonLabel), panel, white)
				textTheme(im, 240, 337, 2, strings.ToUpper(v.Hint), grey, true, m.Appearance)
			} else {
				textTheme(im, 240, 264, 3, strings.ToUpper(v.Secondary), grey, true, m.Appearance)
			}
		}
		button(im, 240, 398, 140, 42, "HOME", panel, white)
	}
	drawWidgetOverlays(im, m)
	if m.Banner != nil && now.Before(m.BannerUntil) && m.Page != NotificationCenter {
		rect(im, 95, 66, 385, 130, panel)
		title := strings.ToUpper(m.Banner.Request.Title)
		if len(title) > 21 {
			title = title[:21]
		}
		textTheme(im, 240, 83, 2, title, accent, true, m.Appearance)
		textTheme(im, 240, 108, 1, "NOTIFICATION", white, true, m.Appearance)
	} else if len(m.Notices) > 0 && m.Page != NotificationCenter {
		disc(im, 336, 76, 7, accent)
	}
	if m.Critical != nil {
		disc(im, 240, 240, 167, panel)
		title := strings.ToUpper(m.Critical.Request.Title)
		if len(title) > 20 {
			title = title[:20]
		}
		textTheme(im, 240, 211, 2, title, white, true, m.Appearance)
		textTheme(im, 240, 246, 2, "SYSTEM ALERT", accent, true, m.Appearance)
		button(im, 240, 302, 164, 52, "ACKNOWLEDGE", panel, white)
	}
	if m.TouchUntil.After(now) && within(m.LastTouch.X, m.LastTouch.Y, 220) {
		ring(im, m.LastTouch.X, m.LastTouch.Y, 19, 3, accent)
	}
}
func within(x, y, r int) bool { dx, dy := x-240, y-240; return dx*dx+dy*dy <= r*r }
func disc(im *image.RGBA, cx, cy, r int, c color.RGBA) {
	for y := max(0, cy-r); y < min(Height, cy+r+1); y++ {
		dy := y - cy
		dx := int(math.Sqrt(float64(r*r - dy*dy)))
		for x := max(0, cx-dx); x <= min(Width-1, cx+dx); x++ {
			im.SetRGBA(x, y, c)
		}
	}
}
func ring(im *image.RGBA, cx, cy, r, t int, c color.RGBA) {
	outer := r * r
	inner := (r - t) * (r - t)
	for y := max(0, cy-r); y < min(Height, cy+r+1); y++ {
		for x := max(0, cx-r); x < min(Width, cx+r+1); x++ {
			dx, dy := x-cx, y-cy
			d := dx*dx + dy*dy
			if d <= outer && d >= inner {
				im.SetRGBA(x, y, c)
			}
		}
	}
}
func rect(im *image.RGBA, x1, y1, x2, y2 int, c color.RGBA) {
	for y := max(0, y1); y < min(Height, y2); y++ {
		for x := max(0, x1); x < min(Width, x2); x++ {
			im.SetRGBA(x, y, c)
		}
	}
}
func outlineRect(im *image.RGBA, x1, y1, x2, y2 int, c color.RGBA) {
	rect(im, x1, y1, x2, y1+3, c)
	rect(im, x1, y2-3, x2, y2, c)
	rect(im, x1, y1, x1+3, y2, c)
	rect(im, x2-3, y1, x2, y2, c)
}
func button(im *image.RGBA, cx, cy, w, h int, label string, panel, colorText color.RGBA) {
	rect(im, cx-w/2, cy-h/2, cx+w/2, cy+h/2, panel)
	text(im, cx, cy-10, 3, label, colorText, true)
}

// textTheme uses the system's text scale for all shell-owned legacy views,
// automatically fitting labels inside the circular safe chord.
func textTheme(im *image.RGBA, x, y, base int, label string, c color.RGBA, center bool, prefs appearance.Settings) {
	size := base
	if prefs.TextScale >= 125 && base <= 4 {
		size++
	}
	if prefs.TextScale >= 150 && base > 4 {
		size++
	}
	dy := float64(y + size*4 - 240)
	limit := float64(218*218) - dy*dy
	if limit > 0 {
		half := int(math.Sqrt(limit)) - 7
		width := 2 * half
		if !center {
			width = 240 + half - x
		} else if x != 240 {
			width = 2 * min(x-(240-half), 240+half-x)
		}
		for size > 1 && (len([]rune(label))*6-1)*size > width {
			size--
		}
	}
	text(im, x, y, size, label, c, center)
}

// A small built-in pixel alphabet avoids shipping a font dependency for the hardware smoke test.
var glyph = map[rune][7]string{
	'A':  {"01110", "10001", "10001", "11111", "10001", "10001", "10001"},
	'B':  {"11110", "10001", "10001", "11110", "10001", "10001", "11110"},
	'C':  {"01111", "10000", "10000", "10000", "10000", "10000", "01111"},
	'D':  {"11110", "10001", "10001", "10001", "10001", "10001", "11110"},
	'E':  {"11111", "10000", "10000", "11110", "10000", "10000", "11111"},
	'F':  {"11111", "10000", "10000", "11110", "10000", "10000", "10000"},
	'G':  {"01111", "10000", "10000", "10111", "10001", "10001", "01111"},
	'H':  {"10001", "10001", "10001", "11111", "10001", "10001", "10001"},
	'I':  {"11111", "00100", "00100", "00100", "00100", "00100", "11111"},
	'J':  {"00111", "00010", "00010", "00010", "10010", "10010", "01100"},
	'K':  {"10001", "10010", "10100", "11000", "10100", "10010", "10001"},
	'L':  {"10000", "10000", "10000", "10000", "10000", "10000", "11111"},
	'M':  {"10001", "11011", "10101", "10101", "10001", "10001", "10001"},
	'N':  {"10001", "11001", "10101", "10011", "10001", "10001", "10001"},
	'O':  {"01110", "10001", "10001", "10001", "10001", "10001", "01110"},
	'P':  {"11110", "10001", "10001", "11110", "10000", "10000", "10000"},
	'Q':  {"01110", "10001", "10001", "10001", "10101", "10010", "01101"},
	'R':  {"11110", "10001", "10001", "11110", "10100", "10010", "10001"},
	'S':  {"01111", "10000", "10000", "01110", "00001", "00001", "11110"},
	'T':  {"11111", "00100", "00100", "00100", "00100", "00100", "00100"},
	'U':  {"10001", "10001", "10001", "10001", "10001", "10001", "01110"},
	'V':  {"10001", "10001", "10001", "10001", "10001", "01010", "00100"},
	'W':  {"10001", "10001", "10001", "10101", "10101", "10101", "01010"},
	'X':  {"10001", "10001", "01010", "00100", "01010", "10001", "10001"},
	'Y':  {"10001", "10001", "01010", "00100", "00100", "00100", "00100"},
	'Z':  {"11111", "00001", "00010", "00100", "01000", "10000", "11111"},
	'0':  {"01110", "10001", "10011", "10101", "11001", "10001", "01110"},
	'1':  {"00100", "01100", "00100", "00100", "00100", "00100", "01110"},
	'2':  {"01110", "10001", "00001", "00010", "00100", "01000", "11111"},
	'3':  {"11110", "00001", "00001", "01110", "00001", "00001", "11110"},
	'4':  {"00010", "00110", "01010", "10010", "11111", "00010", "00010"},
	'5':  {"11111", "10000", "10000", "11110", "00001", "00001", "11110"},
	'6':  {"00111", "01000", "10000", "11110", "10001", "10001", "01110"},
	'7':  {"11111", "00001", "00010", "00100", "01000", "01000", "01000"},
	'8':  {"01110", "10001", "10001", "01110", "10001", "10001", "01110"},
	'9':  {"01110", "10001", "10001", "01111", "00001", "00010", "11100"},
	':':  {"00000", "00100", "00100", "00000", "00100", "00100", "00000"},
	'.':  {"00000", "00000", "00000", "00000", "00000", "01100", "01100"},
	',':  {"00000", "00000", "00000", "00000", "01100", "01100", "01000"},
	'!':  {"00100", "00100", "00100", "00100", "00100", "00000", "00100"},
	'?':  {"01110", "10001", "00001", "00010", "00100", "00000", "00100"},
	'\'': {"00100", "00100", "00010", "00000", "00000", "00000", "00000"},
	'"':  {"01010", "01010", "00000", "00000", "00000", "00000", "00000"},
	'`':  {"01000", "00100", "00000", "00000", "00000", "00000", "00000"},
	'[':  {"01110", "01000", "01000", "01000", "01000", "01000", "01110"},
	']':  {"01110", "00010", "00010", "00010", "00010", "00010", "01110"},
	'<':  {"00010", "00100", "01000", "10000", "01000", "00100", "00010"},
	'>':  {"01000", "00100", "00010", "00001", "00010", "00100", "01000"},
	'-':  {"00000", "00000", "00000", "11111", "00000", "00000", "00000"},
	'+':  {"00000", "00100", "00100", "11111", "00100", "00100", "00000"},
	'=':  {"00000", "11111", "00000", "11111", "00000", "00000", "00000"},
	'*':  {"00000", "10101", "01110", "11111", "01110", "10101", "00000"},
	'_':  {"00000", "00000", "00000", "00000", "00000", "00000", "11111"},
	'/':  {"00001", "00001", "00010", "00100", "01000", "10000", "10000"},
	'@':  {"01110", "10001", "10111", "10101", "10111", "10000", "01111"},
	'#':  {"01010", "11111", "01010", "01010", "11111", "01010", "00000"},
	'$':  {"00100", "01111", "10100", "01110", "00101", "11110", "00100"},
	'%':  {"10001", "10010", "00100", "01000", "10001", "00000", "00000"},
	'&':  {"01100", "10010", "10100", "01000", "10101", "10010", "01101"},
	'(':  {"00010", "00100", "01000", "01000", "01000", "00100", "00010"},
	')':  {"01000", "00100", "00010", "00010", "00010", "00100", "01000"},
	';':  {"00000", "00100", "00100", "00000", "00100", "00100", "01000"},
	'|':  {"00100", "00100", "00100", "00100", "00100", "00100", "00100"},
	' ':  {},
}

func text(im *image.RGBA, x, y, scale int, s string, c color.RGBA, center bool) {
	runes := []rune(s)
	total := (len(runes)*6 - 1) * scale
	if center {
		x -= total / 2
	}
	for _, r := range runes {
		rows, ok := glyph[r]
		if !ok {
			rows = glyph[' ']
		}
		for ry, row := range rows {
			for rx, mark := range row {
				if mark == '1' {
					rect(im, x+rx*scale, y+ry*scale, x+(rx+1)*scale, y+(ry+1)*scale, c)
				}
			}
		}
		x += 6 * scale
	}
}
