package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"image/png"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"roundos/internal/appearance"
	"roundos/internal/appmgr"
	"roundos/internal/backlight"
	"roundos/internal/drm"
	"roundos/internal/gesture"
	"roundos/internal/notify"
	"roundos/internal/perf"
	"roundos/internal/presenter"
	"roundos/internal/release"
	"roundos/internal/rotary"
	"roundos/internal/service"
	"roundos/internal/shell"
	"roundos/internal/systemgestures"
	"roundos/internal/touch"
	"roundos/internal/update"
	"roundos/internal/wifi"
	"roundos/internal/wire"
	"roundos/sdk/ui"
)

type action struct {
	wifiStatus wifi.Status
	wifiKind   string
	wifiErr    error
	kind       string
	delta      int
	frame      touch.Frame
}

func main() {
	exitCode := 0
	// Defer the exit until after DRM presenter, application manager and
	// backlight cleanup have completed. systemd only restarts on failure.
	defer func() {
		if exitCode != 0 {
			os.Exit(exitCode)
		}
	}()
	versionFlag := flag.Bool("version", false, "print RoundOS version and exit (no device access)")
	startupProtocol := flag.Bool("startup-protocol", false, "print managed startup protocol and exit (no device access)")
	backend := flag.String("backend", "png", "png (safe preview) or drm")
	output := flag.String("output", "roundos-preview.png", "PNG preview path")
	previewPage := flag.String("preview-page", "home", "PNG preview page: home, settings, wifi, clock, clock-options, clock-details, clock-gallery, timer, timer-duration, timer-picker, timer-dialog, touch, notifications, switcher, editor, accessibility")
	previewTheme := flag.String("preview-theme", "dark", "PNG preview theme: dark, light or contrast")
	card := flag.String("card", "/dev/dri/card0", "DRM device")
	connector := flag.String("connector", "DPI-1", "DRM connector")
	touchscreen := flag.String("touch", "auto", "auto-detect touchscreen (default), explicit /dev/input/eventN, or off")
	// Keep the raw touch-to-screen Y mapping that was physically verified for
	// tap hit-testing in dev5.1.2. The dev5.1.3 global inversion caused mirrored
	// tap locations and edge swipes. Some panels may still require an override.
	touchInvertY := flag.Bool("touch-invert-y", false, "mirror all touch Y coordinates (off by default; use only if touch targets are mirrored)")
	encoder := flag.String("encoder", "off", "optional rotary/gpio-keys evdev path or off")
	buttonDevice := flag.String("button", "off", "optional separate gpio-keys evdev path or off")
	encoderRel := flag.Int("encoder-rel", rotary.RelDial, "encoder relative axis: REL_DIAL=7 or REL_WHEEL=8")
	encoderPress := flag.Int("encoder-press-key", rotary.KeyEnter, "evdev key code for encoder push button, default KEY_ENTER=28")
	encoderReverse := flag.Bool("encoder-reverse", false, "reverse rotary direction")
	gestureConfig := flag.String("gesture-config", "", "optional JSON gesture settings (HyperPixel max_contacts=2 profile by default)")
	quietNotifications := flag.Bool("quiet-notifications", false, "suppress noncritical notification banners, keeping history")
	metricsEvery := flag.Duration("metrics-every", 0, "periodically log input-to-present, frame, and app RSS metrics (0 disables)")
	gestureLog := flag.Bool("gesture-log", false, "log completed gestures (not high-frequency drag/pinch motion)")
	gestureLogMoves := flag.Bool("gesture-log-moves", false, "log every movement gesture; can slow SSH diagnostics")
	touchTrace := flag.Bool("touch-trace", false, "diagnostic only: log rate-limited raw contact positions and counts (may slow input)")
	clockExe := flag.String("clock-exec", "./round-clock", "clock app executable")
	timerExe := flag.String("timer-exec", "./round-timer", "timer app executable")
	appsDir := flag.String("apps-dir", "/etc/roundos/apps", "trusted optional-app manifest directory (missing directory is allowed)")
	ruiDevApp := flag.String("rui-dev-app", "", "manual development only: registered app receiving compiled layout reloads")
	ruiDevDir := flag.String("rui-dev-dir", "", "manual development only: app-owned private directory for reload control")
	stateDir := flag.String("state-dir", "", "directory for shell-owned state (appearance, etc.)")
	appUser := flag.String("app-user", "", "dedicated non-root system user for all apps (required when roundd runs as root)")
	appStateDir := flag.String("app-state-dir", "/var/lib/roundos/app-data", "pre-provisioned private app data directory")
	appearancePath := flag.String("appearance-path", "", "optional appearance preferences JSON file; defaults to state directory")
	backlightDevice := flag.String("backlight", "off", "off (default), auto, or explicit /sys/class/backlight/<name> path")
	idleOff := flag.Duration("idle-off", 0, "optional inactivity timeout; requires -backlight, e.g. 45s")
	flag.Parse()
	if (*ruiDevApp == "") != (*ruiDevDir == "") {
		log.Fatal("both -rui-dev-app and -rui-dev-dir are required")
	}
	if *ruiDevApp != "" && (os.Getenv("NOTIFY_SOCKET") != "" || os.Getenv("INVOCATION_ID") != "") {
		log.Fatal("RUI development is unavailable in managed service sessions")
	}
	if *versionFlag {
		fmt.Printf("RoundOS %s (%s)\n", release.Version, release.BuildID)
		return
	}
	if *startupProtocol {
		fmt.Println(service.StartupProtocol)
		return
	}
	if *metricsEvery < 0 {
		log.Fatal("-metrics-every cannot be negative")
	}
	cfg, err := systemgestures.Load(*gestureConfig)
	fatalIf(err)
	router := systemgestures.New(cfg)
	log.Printf("gesture hardware profile: max_contacts=%d, two-finger switch=%t, launcher=%t, vertical_two_swipe_reverse=%t", cfg.MaxContacts, cfg.TwoSwitch, cfg.TwoLauncher, cfg.TwoVerticalReverse)
	if cfg.MaxContacts < 3 && (cfg.ThreeSwitch || cfg.ThreeLauncher || cfg.FourSwitcher || cfg.FiveApp) {
		log.Printf("gesture hardware profile: unsupported 3+/4+/5-contact shortcuts will not activate")
	}
	model := shell.New()
	extraApps, err := appmgr.Discover(*appsDir)
	fatalIf(err)
	model.SetInstalledApps(extraApps)
	if len(extraApps) > 0 {
		log.Printf("registered %d additional RoundOS applications", len(extraApps))
	}
	model.SupportedContacts = cfg.MaxContacts
	notificationStore := notify.New()
	model.Notifier = notificationStore
	notificationStore.SetQuietMode(*quietNotifications)
	if *backend == "png" {
		ps := model.Appearance
		ps.Theme = *previewTheme
		if err := ps.Valid(); err != nil {
			log.Fatal(err)
		}
		model.Appearance = ps
		if *previewPage == "clock" || *previewPage == "clock-options" || *previewPage == "clock-details" || *previewPage == "clock-gallery" {
			model.SetView(wire.ClockID, wire.View{Title: "CLOCK", Primary: "10:08", Screens: []wire.Screen{
				ui.Screen("clock.root", "CLOCK", "10:08", "WED NOV 11", ui.PrimaryButton("clock.label", "EDIT LABEL", "edit_label"), ui.Link("clock.options", "OPTIONS", "clock.options")),
				ui.Screen("clock.options", "CLOCK OPTIONS", "", "", ui.Toggle("clock.24hour", "24-HOUR CLOCK", "toggle_24hour", true), ui.Link("clock.details", "PICKER EXAMPLES", "clock.details"), ui.Link("clock.gallery", "WIDGET GALLERY", "clock.gallery"), ui.ListItem("clock.help", "HELP", "HOW TO USE", "clock_help"), ui.Link("clock.about", "ABOUT", "clock.about")),
				ui.Screen("clock.details", "DETAILS", "", "", ui.TimePicker("clock.alarm", "ALARM TIME", "set_alarm_time", 480, 5), ui.NumberPicker("clock.snooze", "NUMBER EXAMPLE", "set_snooze", 5, 1, 30, 1), ui.DatePickerDays("clock.date", "DATE EXAMPLE", "set_date", int(time.Now().UTC().Unix()/86400), int(time.Now().UTC().Unix()/86400)-365, int(time.Now().UTC().Unix()/86400)+365)),
				ui.Screen("clock.gallery", "WIDGET GALLERY", "", "SCROLL TO EXPLORE", ui.ListItem("g1", "ITEM ONE", "DEMO", "example"), ui.ListItem("g2", "ITEM TWO", "DEMO", "example"), ui.ListItem("g3", "ITEM THREE", "DEMO", "example"), ui.ListItem("g4", "ITEM FOUR", "DEMO", "example"), ui.ListItem("g5", "ITEM FIVE", "DEMO", "example"), ui.ListItem("g6", "ITEM SIX", "DEMO", "example"), ui.ListItem("g7", "ITEM SEVEN", "DEMO", "example"), ui.ListItem("g8", "ITEM EIGHT", "DEMO", "example")),
				ui.Screen("clock.about", "ABOUT", "", "LOCAL CLOCK", ui.Label("clock.local", "LOCAL")),
			}})
			model.Select(shell.Clock)
			if *previewPage == "clock-options" || *previewPage == "clock-details" || *previewPage == "clock-gallery" {
				model.Rotate(1)
				model.Activate()
				if *previewPage == "clock-details" {
					model.Rotate(1)
					model.Activate()
				} else if *previewPage == "clock-gallery" {
					model.Rotate(2)
					model.Activate()
				}
			}
		} else if *previewPage == "timer" || *previewPage == "timer-duration" || *previewPage == "timer-picker" || *previewPage == "timer-dialog" {
			model.SetView(wire.TimerID, wire.View{Title: "TIMER", Primary: "04:38", Screens: []wire.Screen{
				ui.Screen("timer.root", "TIMER", "04:38", "COUNTING DOWN", ui.PrimaryButton("timer.toggle", "PAUSE", "toggle"), ui.Link("timer.duration.link", "DURATION", "timer.duration")),
				ui.Screen("timer.duration", "DURATION", "", "STOP TIMER TO EDIT", ui.Slider("timer.minutes", "MINUTES", "set_minutes", 5, 0, 999, 1), ui.NumberPicker("timer.picker", "SET MINUTES", "set_minutes", 5, 0, 999, 1), ui.Button("timer.reset", "RESET TIMER", "request_reset"), ui.Progress("timer.progress", "TIME REMAINING", 5, 0, 999)),
			}})
			model.Select(shell.Timer)
			if *previewPage != "timer" {
				model.Rotate(1)
				model.Activate()
				if *previewPage == "timer-picker" {
					model.Rotate(1)
					model.Activate()
				}
				if *previewPage == "timer-dialog" {
					v := model.Views[shell.Timer]
					v.Dialog = ui.ConfirmDialog("timer-reset", "RESET TIMER?", "THIS WILL DISCARD TIME", "confirm_reset", "cancel_reset", true)
					model.SetView(wire.TimerID, v)
				}
			}
		} else if *previewPage == "settings" {
			model.OpenSettings()
		} else if *previewPage == "wifi" {
			model.OpenSettings()
			model.OpenWiFi()
			model.WiFiResult(wifi.Status{Interface: "wlan0", Current: "RoundOS Lab", Networks: []wifi.Network{{SSID: "RoundOS Lab", Signal: 91, Security: "WPA2", Connected: true}, {SSID: "Guest", Signal: 74, Security: "--"}, {SSID: "Studio", Signal: 45, Security: "WPA2"}}}, "scan", nil)
			model.WiFi.Focus = 1
		} else if *previewPage == "touch" {
			model.Select(shell.Diagnostics)
			model.UpdateContacts([]touch.Contact{
				{ID: 11, Slot: 0, X: 195, Y: 210},
				{ID: 12, Slot: 1, X: 290, Y: 240},
			})
			model.SetGesture("two_contacts_hyperpixel")
		} else if *previewPage == "notifications" {
			now := time.Now()
			r := notificationStore.Emit(wire.TimerID, notify.Request{Title: "Timer finished", Body: "Countdown completed", Priority: notify.Actionable, Action: "open"}, now, false)
			notificationStore.Emit(wire.ClockID, notify.Request{Title: "Clock update", Priority: notify.Quiet}, now, false)
			model.UpdateNotices(notificationStore.Items(), r, now)
			model.OpenNotifications()
		} else if *previewPage == "switcher" {
			model.OpenSwitcher()
		} else if *previewPage == "editor" {
			model.OpenEditor()
		} else if *previewPage == "accessibility" {
			model.Select(shell.Accessibility)
		} else if *previewPage != "home" {
			log.Fatalf("unsupported preview page %q", *previewPage)
		}
		f, err := os.Create(*output)
		fatalIf(err)
		defer f.Close()
		fatalIf(png.Encode(f, shell.Draw(model, time.Now())))
		fmt.Printf("Saved safe 480x480 preview to %s\n", *output)
		return
	}
	if *backend != "drm" {
		log.Fatal("backend must be png or drm")
	}
	if *stateDir == "" {
		home, err := os.UserHomeDir()
		fatalIf(err)
		*stateDir = filepath.Join(home, ".local", "state", "roundos")
	}
	if *appearancePath == "" {
		*appearancePath = filepath.Join(*stateDir, "appearance.json")
	}
	var appRuntime *appmgr.Runtime
	if os.Geteuid() == 0 && *appUser == "" {
		log.Fatal("root RoundOS must use -app-user roundos-app; run scripts/setup-app-user.sh first")
	}
	if *appUser != "" {
		appRuntime = &appmgr.Runtime{User: *appUser, StateDir: *appStateDir}
		fatalIf(appmgr.CheckRuntime(appRuntime))
		log.Printf("application isolation: non-root user=%s (shared app data directory)", *appUser)
	}
	model.Appearance = appearance.Load(*appearancePath)
	log.Printf("appearance loaded: theme=%s text_scale=%d reduce_motion=%t", model.Appearance.Theme, model.Appearance.TextScale, model.Appearance.ReduceMotion)
	display, err := drm.Open(*card, *connector)
	fatalIf(err)
	defer display.Close()
	var light *backlight.Controller
	if *backlightDevice != "off" {
		light, err = backlight.Open(*backlightDevice)
		if err != nil {
			log.Printf("backlight disabled: %v", err)
		} else {
			log.Printf("backlight controller: %s; idle timeout %s", light.Name(), *idleOff)
			defer func() {
				if err := light.Restore(); err != nil {
					log.Printf("restore backlight: %v", err)
				}
			}()
		}
	}
	if *idleOff < 0 {
		log.Fatal("-idle-off cannot be negative")
	}
	if *idleOff > 0 && light == nil {
		log.Print("idle screen-off not active: no kernel backlight controller")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	updateStore, updateErr := update.Open(filepath.Join(*stateDir, "updates.json"), time.Now())
	if updateErr != nil {
		log.Printf("update state recovery: %v", updateErr)
	}
	updates := update.Start(ctx, updateStore, update.NewGitHub(), release.Version, nil)
	updateEvents := updates.Events()
	var dev *appmgr.DevConfig
	if *ruiDevApp != "" {
		dev = &appmgr.DevConfig{AppID: *ruiDevApp, Directory: *ruiDevDir}
	}
	manager, err := appmgr.NewWithAppsRuntimeDev(ctx, *clockExe, *timerExe, *stateDir, extraApps, appRuntime, dev)
	fatalIf(err)
	defer manager.Close()
	manager.Log()
	actions := make(chan action, 128)
	if *touchscreen != "off" {
		resolved := *touchscreen
		if resolved == "auto" {
			info, discoveryErr := touch.DiscoverDevice()
			if discoveryErr != nil {
				log.Printf("touch auto-discovery failed: %v", discoveryErr)
				resolved = ""
			} else {
				resolved = info.Path
				log.Printf("touch auto-discovered: %s (%s), multitouch=%t", info.Path, info.Name, info.Multitouch)
			}
		}
		var reader *touch.Reader
		if resolved != "" {
			reader, err = touch.OpenWithOrientation(resolved, *touchInvertY)
		} else {
			err = fmt.Errorf("no touchscreen selected; inspect discovery log or pass -touch /dev/input/eventN")
		}
		if err != nil {
			log.Printf("touch unavailable (%v); SSH controls available", err)
		} else {
			log.Print(reader.Description())
			defer reader.Close()
			frames := make(chan touch.Frame, 64)
			go func() {
				if err := reader.Run(ctx, frames); err != nil && ctx.Err() == nil {
					log.Printf("touch reader: %v", err)
				}
			}()
			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					case f := <-frames:
						select {
						case actions <- action{kind: "touch_frame", frame: f}:
						case <-ctx.Done():
							return
						}
					}
				}
			}()
		}
	}
	if *encoder != "off" {
		cfg := rotary.DefaultConfig()
		cfg.RelCode = *encoderRel
		cfg.PressKey = *encoderPress
		cfg.Reverse = *encoderReverse
		er, err := rotary.Open(*encoder, cfg)
		if err != nil {
			log.Printf("rotary unavailable (%v); SSH l/r/p still available", err)
		} else {
			defer er.Close()
			inputs := make(chan rotary.Event, 64)
			go func() {
				if err := er.Run(ctx, inputs); err != nil && ctx.Err() == nil {
					log.Printf("rotary reader: %v", err)
				}
			}()
			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					case ev := <-inputs:
						select {
						case actions <- action{kind: ev.Kind, delta: ev.Delta}:
						case <-ctx.Done():
							return
						}
					}
				}
			}()
			log.Printf("rotary input enabled: %s (REL=%d press key=%d)", *encoder, *encoderRel, *encoderPress)
		}
	}
	if *buttonDevice != "off" && *buttonDevice != *encoder {
		cfg := rotary.DefaultConfig()
		cfg.RelCode = -1
		cfg.UpKey = -1
		cfg.DownKey = -1
		cfg.PressKey = *encoderPress
		br, err := rotary.Open(*buttonDevice, cfg)
		if err != nil {
			log.Printf("push button unavailable (%v)", err)
		} else {
			defer br.Close()
			inputs := make(chan rotary.Event, 32)
			go func() {
				if err := br.Run(ctx, inputs); err != nil && ctx.Err() == nil {
					log.Printf("button reader: %v", err)
				}
			}()
			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					case ev := <-inputs:
						select {
						case actions <- action{kind: ev.Kind, delta: ev.Delta}:
						case <-ctx.Done():
							return
						}
					}
				}
			}()
			log.Printf("push button enabled: %s (key=%d)", *buttonDevice, *encoderPress)
		}
	}
	go stdinCommands(ctx, actions)
	log.Printf("RoundOS %s release: %s, %s. SSH: l/r/p/b/h/c/t/w/g/k/n/e/z/na/nq/nc/a/x/s/q + Enter", release.Version, *card, *connector)
	// Rendering/DRM page flips run on a separate goroutine. Input and gesture
	// processing never wait for the display's ~16.7 ms refresh interval.
	// A capacity-one mailbox always retains the newest immutable UI snapshot.
	metrics := perf.New()
	defer func() { log.Printf("RoundOS stats: %s", metrics.Summary()) }()
	var metricsTick <-chan time.Time
	if *metricsEvery > 0 {
		ticker := time.NewTicker(*metricsEvery)
		defer ticker.Stop()
		metricsTick = ticker.C
	}
	presenterLoop := presenter.New(display, metrics)
	defer presenterLoop.Close() // must happen before display.Close()
	startup := service.NewStartup(service.NotifyReady)
	firstFrame := presenterLoop.FirstFrame()
	render := func(inputAt time.Time) {
		presenterLoop.Submit(model, inputAt)
	}
	render(time.Time{})
	flushText := func() {
		appID, result := model.TakeTextResult()
		if result != nil && !manager.TextResult(appID, *result) {
			log.Printf("text result undeliverable to %s", appID)
		}
	}
	wifiClient := wifi.New()
	flushWiFi := func() {
		req := model.TakeWiFiRequest()
		if req == nil {
			return
		}
		go func(r *shell.WiFiRequest) {
			ctxOp, cancel := context.WithTimeout(ctx, 55*time.Second)
			defer cancel()
			var status wifi.Status
			var err error
			switch r.Kind {
			case "scan":
				status, err = wifiClient.Scan(ctxOp)
			case "connect":
				err = wifiClient.Connect(ctxOp, r.Interface, r.Network, r.Password)
			case "disconnect":
				err = wifiClient.Disconnect(ctxOp, r.Interface)
			default:
				err = fmt.Errorf("invalid Wi-Fi operation")
			}
			// The completed operation is delivered to the shell UI event loop.
			select {
			case actions <- action{kind: "wifi_result", wifiStatus: status, wifiKind: r.Kind, wifiErr: err}:
			case <-ctx.Done():
			}
		}(req)
	}
	flushDialog := func() {
		appID, action := model.TakeDialogAction()
		if action != "" && !manager.Input(appID, action, 0) {
			log.Printf("dialog action undeliverable: %s", appID)
		}
	}
	lastInput := time.Now()
	backlightOff := false
	gestures := gesture.New()
	suppressTouch := false
	var lastTouchTrace time.Time
	lastTouchCount := -1
	suppressButton := false
	var buttonDownAt time.Time
	buttonStage := 0
	handleGesture := func(ev gesture.Event) bool {
		if *gestureLog && (*gestureLogMoves || ev.Type == gesture.Tap || ev.Type == gesture.DoubleTap || ev.Type == gesture.LongPress || ev.Type == gesture.Swipe || ev.Type == gesture.TwoSwipe || ev.Type == gesture.ThreeSwipe || ev.Type == gesture.FourTap || ev.Type == gesture.DragEnd || ev.Type == gesture.Cancel) {
			log.Printf("gesture %-13s fingers=%d at=(%.0f,%.0f) delta=(%.0f,%.0f) scale=%.2f angle=%.1f direction=%s", ev.Type, ev.Fingers, ev.X, ev.Y, ev.DX, ev.DY, ev.Scale, ev.Angle, ev.Direction)
		}
		changed := model.SetGesture(string(ev.Type))
		target := model.AppID(model.Page)
		if ev.Type == gesture.FiveContacts && (!cfg.FiveApp || cfg.MaxContacts < 5) {
			return changed
		}
		if model.HasCritical() {
			if ev.Type == gesture.Tap && ev.X >= 148 && ev.X <= 332 && ev.Y >= 276 && ev.Y <= 336 {
				return model.AcknowledgeCritical() || changed
			}
			return changed
		}
		if action := router.Handle(ev); action != systemgestures.None && !(model.Page == shell.Diagnostics && (ev.Type == gesture.TwoSwipe || ev.Type == gesture.ThreeSwipe || ev.Type == gesture.FourTap)) {
			if *gestureLog {
				log.Printf("system gesture routed: %s (page=%d)", action, model.Page)
			}
			// An edge gesture may begin as an application drag. Cancel it
			// before handing its contact sequence to the system shell.
			if target != "" {
				manager.Gesture(target, wire.Gesture{Kind: "cancel", Fingers: ev.Fingers})
			}
			switch action {
			case systemgestures.Back:
				changed = model.Back() || changed
			case systemgestures.Notifications:
				changed = model.OpenNotifications() || changed
			case systemgestures.Launcher:
				changed = model.Select(shell.Home) || changed
			case systemgestures.Switcher:
				changed = model.OpenSwitcher() || changed
			case systemgestures.NextApp:
				changed = model.NextRunning(1) || changed
			case systemgestures.PreviousApp:
				changed = model.NextRunning(-1) || changed
			}
			// Suppress the remaining contacts, so a release cannot activate
			// a widget on the newly selected screen.
			if ev.Type != gesture.FourTap {
				suppressTouch = true
			}
			return changed
		}
		switch ev.Type {
		case gesture.Down:
			model.StopKinetic()
			model.Touch(int(ev.X), int(ev.Y))
			changed = true
		case gesture.Tap:
			if model.Banner != nil && ev.X >= 95 && ev.X <= 385 && ev.Y >= 66 && ev.Y <= 130 {
				appPage := model.PageForApp(model.Banner.App)
				model.Banner = nil
				if appPage != shell.Home {
					return model.Select(appPage) || true
				}
				return model.OpenNotifications() || true
			}
			c, appAction := model.Tap(int(ev.X), int(ev.Y))
			changed = changed || c
			if appAction != "" {
				manager.Input(target, appAction, model.WidgetDelta)
			}
		case gesture.DragStart:
			originX := int(ev.X - ev.DX)
			originY := int(ev.Y - ev.DY)
			changed = model.WidgetDragStart(originX, originY) || changed
			changed = model.WidgetDragMoveAt(int(ev.X), int(ev.Y), ev.DY, ev.At) || changed
		case gesture.Drag:
			changed = model.WidgetDragMoveAt(int(ev.X), int(ev.Y), ev.DY, ev.At) || changed
		case gesture.Swipe:
			changed = model.WiFiSwipe(ev.Direction) || changed
		case gesture.DragEnd:
			var appAction string
			var updated bool
			updated, appAction = model.WidgetDragEndAt(ev.At)
			changed = updated || changed
			if appAction != "" {
				manager.Input(target, appAction, model.WidgetDelta)
			}
		case gesture.Cancel:
			model.StopKinetic()
			model.CancelWidgetDrag()
		case gesture.TwoSwipe:
			// If not claimed by the optional two-contact system shortcuts,
			// a two-finger swipe belongs to the foreground application.
		}
		if target != "" {
			manager.Gesture(target, wire.Gesture{Kind: string(ev.Type), X: ev.X, Y: ev.Y, DX: ev.DX, DY: ev.DY, Scale: ev.Scale, Angle: ev.Angle, Fingers: ev.Fingers, Direction: ev.Direction})
		}
		return changed
	}
	for {
		now := time.Now()
		var timer *time.Timer
		var alarm <-chan time.Time
		var next time.Duration
		if !backlightOff {
			next = nextWake(model, now)
			if deadline := gestures.Deadline(); !deadline.IsZero() {
				delay := deadline.Sub(now)
				if delay <= 0 {
					delay = time.Millisecond
				}
				if next == 0 || delay < next {
					next = delay
				}
			}
			if !buttonDownAt.IsZero() && buttonStage < 2 {
				threshold := 700 * time.Millisecond
				if buttonStage == 1 {
					threshold = 2 * time.Second
				}
				delay := buttonDownAt.Add(threshold).Sub(now)
				if delay <= 0 {
					delay = time.Millisecond
				}
				if next == 0 || delay < next {
					next = delay
				}
			}
			if light != nil && *idleOff > 0 {
				remaining := lastInput.Add(*idleOff).Sub(now)
				if remaining <= 0 {
					remaining = time.Millisecond
				}
				if next == 0 || remaining < next {
					next = remaining
				}
			}
		}
		if next > 0 {
			timer = time.NewTimer(next)
			alarm = timer.C
		}
		select {
		case <-firstFrame:
			if timer != nil {
				timer.Stop()
			}
			firstFrame = nil
			if err := startup.FramePresented(); err != nil {
				log.Printf("startup readiness notification failed: %v", err)
				exitCode = 1
				return
			}
		case <-metricsTick:
			log.Printf("RoundOS performance: %s; coalesced-visual-frames=%d", metrics.Summary(), presenterLoop.Coalesced())
			if rss, err := perf.RSS(os.Getpid()); err == nil {
				log.Printf("roundd RSS: %.2f MiB", float64(rss)/(1024*1024))
			}
			for appID, pid := range manager.PIDs() {
				if rss, err := perf.RSS(pid); err == nil {
					log.Printf("%s PID=%d RSS=%.2f MiB", appID, pid, float64(rss)/(1024*1024))
				}
			}
		case status, ok := <-updateEvents:
			if !ok {
				updateEvents = nil
				continue
			}
			if status.OperationErr != nil {
				log.Printf("release update operation: %v", status.OperationErr)
			} else if status.Err != nil {
				log.Printf("release check: %v", status.Err)
			} else if status.Sequence > 0 && !status.Busy {
				log.Printf("release check: %s", status.Result.Status)
			}
		case err := <-presenterLoop.Errors():
			if timer != nil {
				timer.Stop()
			}
			log.Printf("DRM presenter failed: %v", err)
			exitCode = 1
			return
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case a := <-actions:
			if timer != nil {
				timer.Stop()
			}
			inputAt := time.Now()
			if a.kind == "touch_frame" && !a.frame.At.IsZero() {
				inputAt = a.frame.At // timestamp when the complete evdev frame was decoded
			}
			if a.kind == "stats" {
				log.Printf("RoundOS stats: %s; coalesced-visual-frames=%d", metrics.Summary(), presenterLoop.Coalesced())
				if rss, err := perf.RSS(os.Getpid()); err == nil {
					log.Printf("roundd RSS: %.2f MiB", float64(rss)/(1024*1024))
				}
				for id, pid := range manager.PIDs() {
					if rss, err := perf.RSS(pid); err == nil {
						log.Printf("%s PID=%d RSS=%.2f MiB", id, pid, float64(rss)/(1024*1024))
					}
				}
				continue
			}
			interactive := a.kind == "touch_frame" || a.kind == "rotate" || a.kind == "press" || a.kind == "button_down" || a.kind == "home" || a.kind == "clock" || a.kind == "timer" || a.kind == "diagnostics" || a.kind == "notifications" || a.kind == "switcher" || a.kind == "editor" || a.kind == "accessibility" || a.kind == "settings" || a.kind == "back"
			if interactive {
				lastInput = inputAt
			}
			if backlightOff && interactive {
				if a.kind == "touch_frame" && len(a.frame.Contacts) == 0 {
					continue
				}
				if err := light.On(); err != nil {
					log.Printf("backlight wake failed: %v", err)
				} else {
					backlightOff = false
					if a.kind == "touch_frame" {
						suppressTouch = true
						gestures.Reset()
					}
					if a.kind == "button_down" {
						suppressButton = true
					}
					render(inputAt)
					continue
				}
			}
			old := model.Page
			changed := false
			// Diagnostic notifications use the exact same store/routing as
			// messages from authenticated background app processes.
			if a.kind == "notify_quiet" || a.kind == "notify_actionable" || a.kind == "notify_critical" {
				req := notify.Request{Title: "Weather updated", Body: "Forecast refreshed", Priority: notify.Quiet}
				appID := wire.ClockID
				system := false
				if a.kind == "notify_actionable" {
					req = notify.Request{Title: "Timer finished", Body: "Countdown completed", Priority: notify.Actionable, Action: "open"}
					appID = wire.TimerID
				}
				if a.kind == "notify_critical" {
					req = notify.Request{Title: "Storage warning", Body: "Storage almost full", Priority: notify.Critical}
					appID = "system"
					system = true
				}
				result := notificationStore.Emit(appID, req, inputAt, system)
				changed = model.UpdateNotices(notificationStore.Items(), result, inputAt)
			}
			switch a.kind {
			case "touch_frame":
				if *touchTrace && (len(a.frame.Contacts) != lastTouchCount || a.frame.At.Sub(lastTouchTrace) >= 75*time.Millisecond || a.frame.Canceled) {
					log.Printf("touch trace: contacts=%d positions=%+v canceled=%t", len(a.frame.Contacts), a.frame.Contacts, a.frame.Canceled)
					lastTouchTrace = a.frame.At
					lastTouchCount = len(a.frame.Contacts)
				}
				if suppressTouch {
					if len(a.frame.Contacts) == 0 || a.frame.Canceled {
						suppressTouch = false
						gestures.Reset()
						router.Reset()
						changed = model.UpdateContacts(nil)
					}
				} else {
					changed = model.UpdateContacts(a.frame.Contacts)
					if target := model.AppID(model.Page); target != "" {
						// Optional raw contact snapshots, never blocking the UI loop.
						manager.Contacts(target, a.frame.Contacts)
					}
					for _, ev := range gestures.Process(a.frame) {
						changed = handleGesture(ev) || changed
					}
				}
			case "rotate":
				var act string
				var delta int
				changed, act, delta = model.Rotate(a.delta)
				if act != "" {
					manager.Input(model.AppID(model.Page), act, delta)
				}
			case "press":
				var act string
				changed, act = model.Activate()
				if act != "" {
					manager.Input(model.AppID(model.Page), act, model.WidgetDelta)
				}
			case "button_down":
				buttonDownAt = inputAt
				buttonStage = 0
			case "button_up":
				if suppressButton {
					suppressButton = false
					buttonDownAt = time.Time{}
					break
				}
				if !buttonDownAt.IsZero() {
					elapsed := inputAt.Sub(buttonDownAt)
					if elapsed >= 700*time.Millisecond && model.PickerID != "" && buttonStage == 0 {
						changed = model.CancelPicker()
					} else if elapsed >= 2*time.Second && buttonStage < 2 {
						changed = model.Select(shell.Home)
					} else if elapsed >= 700*time.Millisecond && buttonStage == 0 {
						changed = model.OpenSwitcher()
					} else if elapsed < 700*time.Millisecond && buttonStage == 0 {
						var act string
						changed, act = model.Activate()
						if act != "" {
							manager.Input(model.AppID(model.Page), act, model.WidgetDelta)
						}
					}
				}
				buttonDownAt = time.Time{}
				buttonStage = 0
			case "editor":
				changed = model.OpenEditor()
			case "accessibility":
				changed = model.OpenAccessibility()
			case "settings":
				changed = model.OpenSettings()
			case "wifi_result":
				if a.wifiErr != nil {
					log.Printf("Wi-Fi %s failed: %v", a.wifiKind, a.wifiErr)
				}
				changed = model.WiFiResult(a.wifiStatus, a.wifiKind, a.wifiErr)
			case "back":
				changed = model.Back()
			case "notifications":
				changed = model.OpenNotifications()
			case "switcher":
				changed = model.OpenSwitcher()
			case "acknowledge":
				changed = model.AcknowledgeCritical()
			case "home":
				changed = model.Select(shell.Home)
			case "clock":
				changed = model.Select(shell.Clock)
			case "timer":
				changed = model.Select(shell.Timer)
			case "diagnostics":
				changed = model.Select(shell.Diagnostics)
			case "crash":
				manager.Input(wire.TimerID, "crash", 0)
			case "quit":
				return
			}
			flushText()
			flushDialog()
			flushWiFi()
			if model.AppearanceDirty {
				if err := appearance.Save(*appearancePath, model.Appearance); err != nil {
					log.Printf("appearance save: %v", err)
				}
				model.AppearanceDirty = false
			}
			if old != model.Page {
				gestures.Reset() // Prevent a gesture begun in one app activating another.
				router.Reset()
				if err := manager.SetForeground(model.AppID(model.Page)); err != nil {
					log.Printf("foreground selection failed: %v", err)
				}
			}
			if changed && !backlightOff {
				render(inputAt)
			}
		case ev := <-manager.Events():
			if timer != nil {
				timer.Stop()
			}
			if err := startup.AppEvent(ev.App, ev.Kind); err != nil {
				log.Printf("startup readiness notification failed: %v", err)
				exitCode = 1
				return
			}
			changed := false
			oldPage := model.Page
			oldInputRevision := model.InputRevision
			switch ev.Kind {
			case "notify":
				if ev.Notification != nil {
					r := notify.Request{Title: ev.Notification.Title, Body: ev.Notification.Body, Group: ev.Notification.Group, Priority: notify.Priority(ev.Notification.Priority), Action: ev.Notification.Action}
					result := notificationStore.Emit(ev.App, r, time.Now(), false)
					if result.Item.ID != 0 {
						changed = model.UpdateNotices(notificationStore.Items(), result, time.Now())
					}
				}
			case "text_request":
				if ev.TextRequest != nil {
					changed = model.OpenAppEditor(ev.App, *ev.TextRequest)
					if !changed {
						manager.TextResult(ev.App, wire.TextResult{ID: ev.TextRequest.ID, Canceled: true})
					}
				}
			case "view":
				changed = model.SetView(ev.App, ev.View)
			case "connected":
				changed = model.SetStatus(ev.App, "WAITING FOR APP")
			case "disconnected", "error", "stopped":
				changed = model.SetStatus(ev.App, "APP OFFLINE")
			}
			if ev.Kind == "error" || ev.Kind == "stopped" {
				log.Printf("app %s: %s", ev.App, ev.Detail)
			}
			if oldInputRevision != model.InputRevision {
				suppressTouch = gestures.CancelUntilRelease() || suppressTouch
				router.Reset()
				if target := model.AppID(model.Page); target != "" {
					manager.Gesture(target, wire.Gesture{Kind: "cancel"})
					manager.Contacts(target, nil)
				}
				if !buttonDownAt.IsZero() {
					suppressButton = true
					buttonDownAt = time.Time{}
				}
			}
			if oldPage != model.Page {
				gestures.Reset()
				router.Reset()
				if err := manager.SetForeground(model.AppID(model.Page)); err != nil {
					log.Printf("foreground selection failed: %v", err)
				}
			}
			if changed && !backlightOff {
				render(time.Time{})
			}
		case <-alarm:
			now := time.Now()
			changed := model.AdvanceKinetic(now)
			old := model.Page
			for _, ev := range gestures.Tick(now) {
				changed = handleGesture(ev) || changed
			}
			if !buttonDownAt.IsZero() {
				elapsed := now.Sub(buttonDownAt)
				if elapsed >= 700*time.Millisecond && model.PickerID != "" && buttonStage == 0 {
					buttonStage = 3
					changed = model.CancelPicker() || changed
				} else if elapsed >= 2*time.Second && buttonStage < 2 {
					buttonStage = 2
					changed = model.Select(shell.Home) || changed
				} else if elapsed >= 700*time.Millisecond && buttonStage == 0 {
					buttonStage = 1
					changed = model.OpenSwitcher() || changed
				}
			}
			if old != model.Page {
				gestures.Reset()
				router.Reset()
				if err := manager.SetForeground(model.AppID(model.Page)); err != nil {
					log.Printf("foreground selection failed: %v", err)
				}
			}
			if light != nil && *idleOff > 0 && !backlightOff && time.Since(lastInput) >= *idleOff {
				if err := light.Off(); err != nil {
					log.Printf("backlight off failed; disabling idle-off: %v", err)
					*idleOff = 0
				} else {
					backlightOff = true
					log.Print("backlight off due to inactivity")
				}
				continue
			}
			if model.BannerExpired(now) {
				changed = true
			}
			if !backlightOff && (changed || model.Page == shell.Home || model.TouchUntil.After(now)) {
				render(time.Time{})
			}

		}
	}

}
func nextWake(m *shell.Model, now time.Time) time.Duration {
	next := m.NextKineticWake(now)
	if m.Page == shell.Home {
		next = now.Truncate(time.Minute).Add(time.Minute).Sub(now)
	}
	if m.Banner != nil && m.BannerUntil.After(now) {
		d := m.BannerUntil.Sub(now)
		if next == 0 || d < next {
			next = d
		}
	}
	if m.TouchUntil.After(now) {
		d := m.TouchUntil.Sub(now)
		if next == 0 || d < next {
			next = d
		}
	}
	return next
}
func stdinCommands(ctx context.Context, actions chan<- action) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.ToLower(scanner.Text()))
		a := action{kind: line}
		switch line {
		case "left", "l", "-":
			a.kind = "rotate"
			a.delta = -1
		case "right", "r", "+":
			a.kind = "rotate"
			a.delta = 1
		case "press", "p", "":
			a.kind = "press"
		case "home", "h":
			a.kind = "home"
		case "back", "b":
			a.kind = "back"
		case "editor", "edit", "e":
			a.kind = "editor"
		case "accessibility", "access", "z":
			a.kind = "accessibility"
		case "settings", "w":
			a.kind = "settings"
		case "clock", "c":
			a.kind = "clock"
		case "timer", "t":
			a.kind = "timer"
		case "notifications", "n":
			a.kind = "notifications"
		case "switcher", "k":
			a.kind = "switcher"
		case "nq":
			a.kind = "notify_quiet"
		case "na":
			a.kind = "notify_actionable"
		case "nc":
			a.kind = "notify_critical"
		case "ack", "a":
			a.kind = "acknowledge"
		case "gestures", "diagnostics", "g":
			a.kind = "diagnostics"
		case "crash", "x":
			a.kind = "crash"
		case "stats", "s":
			a.kind = "stats"
		case "quit", "q", "exit":
			a.kind = "quit"
		default:
			fmt.Println("Commands: l/r/p, b (Back), w (Settings), e (Text Edit), z (Accessibility), h/c/t/g, k (switcher), n (notifications), nq/na/nc (simulate notifications), a (ack), x/s/q")
			continue
		}
		select {
		case actions <- a:
		case <-ctx.Done():
			return
		}
	}
}
func fatalIf(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
