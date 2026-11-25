# Milestone 6: Text input and accessibility (0.5-dev6)

This checkpoint extends **0.5-dev5.1.4**, the last physically tested two-contact gesture branch. No kernel, overlay, Plymouth, desktop, or systemd changes. Builds do not overwrite 0.4.1 or earlier RoundOS directories.

## Implemented

- Shell-owned text editor on a 480×480 circular screen with touch QWERTY, shift, Space, Backspace, three character groups, Done, an editable UTF-8 cursor, a maximum of 256 Unicode code points, and a rotary wheel with command actions (`SPACE`, `BKSP`, `LEFT`, `RIGHT`, `SHIFT`, `GROUP`, `DONE`). Touch keys intentionally use smaller targets than ordinary round widgets due to available area.
- App-to-shell text input IPC: an authenticated foreground app can call `Client.RequestText(id,label,initial,secure)` and receives a `text_result` Event with ID, Value, or `Canceled`. This is integrated into Clock's `EDIT LABEL` button for hardware testing. The Clock label is a volatile demonstration value, not stored to disk.
- **Back policy:** First Back while editing hides the keyboard but retains the draft. Second Back, if dirty, prompts for `KEEP` or `DISCARD`, initially focused on KEEP. Rotary and touch access both options. DONE/SAVE commits the field. Ordinary text survives Notification Center interruptions, including returning to the same editor through Back. Secure fields clear on foreground loss and are never saved to preferences.
- Shell-rendered Clock, Timer, Home, switcher, notifications, editor and Settings all use one runtime semantic color palette. Changing theme repaints the active view without relaunching apps. Dark, Light, and High Contrast are supported. Unlike earlier 0.5 milestones, this is still the existing shell-drawn app view, not a generalized public widget-tree/theme SDK.
- Accessibility settings: theme, 100%/125%/150% text scaling, Reduce Motion and Large Touch Targets. The latter changes the Settings list touch hit areas and density. Preferences use validated, atomic JSON saves at `-appearance-path`, defaulting to `<state-dir>/appearance.json` (the test launcher uses sudo; default state-dir is resolved for the invoking root process).
- Launcher: installed apps show icons with names underneath. Focus slides between apps; Reduce Motion switches focus immediately. **WRAP APPS**, under Settings → Appearance (use **PREV** / **NEXT** to browse by touch), defaults to ON, including for existing preferences. Set it to OFF to stop at the first and last app with both rotary and neighbor taps. The value persists in `appearance.json` as `carousel_wrap`.
- Home shows local 24-hour time above the English abbreviated date (for example `Nov. 25, 2026`, rendered in the shell's uppercase font). Both update at the minute boundary. The branding and Touch Test button have been removed from Home; touch diagnostics remain available with `g` + Enter in a manual SSH console session or the explicit `-preview-page touch` PNG preview.
- Touchscreen autodiscovery, correct non-inverted Y touch positions, two-contact gesture profile and vertical swipe compatibility setting remain unchanged from 5.1.4. Existing IPC and nonblocking rendering remain intact.

## Open editor on the Pi

From an SSH terminal inside the RoundOS runtime, enter `c` to show Clock, then tap the central **EDIT LABEL** button (or press the encoder when Clock is focused). The text editor opens. The input you save becomes Clock's hint line. The app process continues running; it is not restarted.

For standalone shell testing, `e` opens the text editor without an app. `z` opens Accessibility Settings. Both are also in the **compact switcher** (`k`, then rotary navigation to **TEXT EDIT** or **ACCESSIBILITY**). `b` simulates Back. `l/r` rotates the editor wheel, `p` inserts the selected character or performs the selected command. With the initial `A` selected, one step left selects `DONE`.

Use `-gesture-config config/gestures.example.json` to test with the verified HyperPixel two-contact shortcuts enabled. From an app or editor, a left-edge inward swipe performs Back and a top-edge down swipe opens notifications.

## Recommended hardware tests

1. From Clock, open **EDIT LABEL**, type letters with the QWERTY keys, use Shift/Space/Backspace, press Done, verify the Clock label changes and its time continues to tick.
2. From Clock, open the editor again, make a change, use visible Back or the left-edge swipe **once**. The keyboard should hide, but the typed value should remain on screen. Use Back **again** and verify the discard confirmation. Test KEEP first, then DISCARD. Verify the old Clock label remains unchanged after discard.
3. With the keyboard visible, use `l/r/p` in SSH to insert a character. Use `l` from the initial A to reach `DONE` and `p` to accept the field; check the Clock result.
4. Type nonsecret text, open Notification Center from the top, then Back. The editor should return with the draft preserved. Don't use an app switch or Home as an implicit commit.
5. Via `z` or the switcher, cycle **THEME** through DARK → LIGHT → CONTRAST and observe the same theme on Clock, Timer, switcher, editor, and settings. Restart RoundOS and confirm the saved theme returns. Restore DARK after testing.
6. Cycle **TEXT SCALE** through 100 → 125 → 150. Check titles and labels remain within the circular boundary. Toggle **LARGE TARGETS** and verify larger Settings row hit areas. On Home, check sliding focus with rotary and neighbor taps, then toggle **REDUCE MOTION** and verify immediate focus changes. Scroll to **WRAP APPS**, disable it, and check that both inputs stop at either end with no opposite-end neighbor displayed. Restart and confirm the preference persists; enable wrapping again and check both boundary transitions.
7. Confirm single-finger top/left edge gestures, two-finger horizontal switches and staggered two-finger upward Home gesture still work. Your panel's physical two-contact recognition is more reliable with vertically staggered fingers.
8. Let Timer run in the background and perform Clock text editing, notification presentation and theme changes. Confirm Timer continues counting down and can recover from the existing simulated crash test.

## Validation performed off-device

- `go test ./...` — PASS (including app IPC integration; before final release rerun in the final package)
- `go test -race ./...` — PASS
- `go vet ./...` — PASS
- Linux armv7 non-DRM cross-compilation — PASS
- 480×480 dark/light/contrast preview images generated
- DRM-enabled (`-tags drm`) ARM build — NOT verified on host (libdrm headers are absent). Build via `./scripts/build.sh` on the Pi.
- Hardware touch, physical screen layout and p95 input latency — NOT yet verified for this checkpoint.

## Known boundaries

- This checkpoint extends the 0.4.1-based milestone-5 branch; prior 0.5-dev1 through 0.5-dev4 widget-tree and carousel rework were not available to merge. The complete circular declarative widget SDK and production-grade accessibility audits are NOT claimed as finished.
- Clock's editable label is a demonstration of the new app API and lasts for that Clock process's lifetime; persistence and an app settings model are separate application features.
- The bitmap test font has limited characters; Unicode is maintained in editor state, but complex scripts need a future real font renderer.
- Text entry is local to the Pi. No external services or secrets are transmitted. Secure fields are not recovered or saved.
- App-switching away from a nonsecret in-progress editor retains the draft in the in-memory shell state, but there is no general editor-session restoration policy across arbitrary app-root changes yet. Notification Center Back restoration is supported.
- Reduce Motion is a stored system preference; the legacy shell has no animated UI transitions, so only later scrollable/animated widgets can consume it.
- The current keyboard is a full-screen field editor, not a complete multiline editor, IME, suggestion/autocorrect service, or universal text layout engine.
