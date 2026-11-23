# RoundOS RUI Milestone M3 — live SDK runtime integration

## Scope and safety

M3 adds `roundos/sdk/rui` and one **opt-in** independent example app. Applying this milestone to current `main` preserves the shell, IPC v7, Clock, Timer, startup recovery, systemd service, boot splash, touch orientation, and DRM driver. The M0 physical lifecycle/performance acceptance remains open, independent of this SDK milestone.

This repository has evolved since the original M2 source archive: CI/release automation and startup recovery intentionally changed some of the 149 original 0.5.0 files. The immutable `scripts/m0/BASELINE-SOURCE.SHA256` remains historical evidence, not a checksum of current `main`. Its verifier retains strict baseline checks and recognizes M3 additions; CI and the M3 script test it using isolated fixtures rather than claiming the current tree is byte-identical to 0.5.0.

**M3 is an additive SDK runtime, not an IPC v8 release.** It consumes the same `sdk/app.Client.Events()` channel as the application, and uses `Client.Publish()` to send existing v7 `View` descriptions. No extra socket reader or renderer is created. RUI compiled layouts are embedded data, not scripts, and must be installed only as part of a reviewed application.

## API and lifecycle

The following code uses only public SDK packages:

```go
//go:embed layout.rui.bin
var layout []byte

client, err := app.Connect()
if err != nil { return err }
defer client.Close()
engine, err := rui.New(client, layout) // verifies bundle AND authenticated app ID
if err != nil { return err }
engine.BindString("display.text", func() string { return model.Label() })
engine.BindInt("timer.minutes", func() int { return model.Minutes() })
engine.BindBool("timer.stopped", func() bool { return !model.Running() })
engine.OnAction("timer.toggle", func(ctx context.Context, ev rui.ActionEvent) error { model.Toggle(); return nil })
engine.OnInt("timer.setMinutes", func(ctx context.Context, minutes int) error { return model.SetMinutes(minutes) })
engine.ValidateBindings() // exact manifest must be satisfied
for ev := range client.Events() {
    if ev.Type == "close" { break }
    handled, err := engine.Dispatch(ev)
    if err != nil { /* report, keep app running */ }
    if !handled { /* legacy gesture/text_result application handlers */ }
    if err := engine.Flush(); err != nil { /* report, retry when resolved */ }
}
```

See [`examples/rui-demo/main.go`](../../examples/rui-demo/main.go) for fully buildable Go source, which uses `//go:embed layout.rui.bin`. The example demonstrates navigation, a toggle, a numeric picker, a live hero label, shell text editor, modal confirmation/cancel, and recovery from visibility transitions.

New APIs:

- `rui.New(publisher, compiledBundle)` reads `ROUNDOS_APP_ID` from the authenticated supervisor environment. Bundle application identity must match. `NewForApp(..., expectedID)` exists for isolated tests/integrators; the app supervisor still authenticates socket and token.
- `BindString`, `BindInt`, `BindBool` install typed state getters. Registration returns errors for undeclared/wrong-type/duplicate names.
- `OnAction`, `OnInt`, `OnBool` register typed handlers, executing synchronously on the caller of `Dispatch`. `OnAction` receives `ActionEvent{Name, Delta}`.
- `ValidateBindings` checks **all** required actions/bindings and freezes registration. Must run before `Dispatch` or `Flush`.
- `Dispatch(app.Event)` consumes `visibility` and declared `input` events, returning `(handled, error)`. Unhandled events—including `text_result`, `gesture`, `contacts`, `close`, and unknown legacy input—remain available to the application.
- `Invalidate(keys ...string)` is goroutine-safe. Empty keys mean whole-tree invalidation; nonempty keys must match a declared binding. Invalidations received while `Flush` is publishing are retained for another flush.
- `Flush()` is serialized and safe against overlapping callers, but should run only on the app event loop. Bound getters must produce a **consistent, data-race-free state snapshot**; synchronize application-owned state accessed by worker goroutines. A failed resolve or publish retains dirty state for retry.
- `ShowDialog(id)`, `HideDialog()` control shell-owned declarative dialogs. Confirm/cancel IPC actions automatically clear the corresponding active dialog before invoking the Go handler.
  Reopening the same dialog from a handler publishes a dialog-free view first, then the reopened dialog, so the v7 shell resets its dismissed state. A failed publication retains this transition for retry.

## Critical IPC v7 semantics

1. **Integer commits carry the absolute value in `Event.Delta`, not a relative offset.** `OnInt` checks declared min/max before calling the handler. Where one integer action is attached to multiple widgets, all must have identical min/max ranges because v7 does not include the originating widget ID.
2. **Toggle actions do not carry a checked state in v7.** `OnBool` computes `!currentBoundState` from the authoritative registered bool getter; the callback must update that state. For reliable live interaction, `checked` must use a bool binding. A literal checked value can be previewed in M2 but is rejected for a live M3 toggle. Multiple controls may share a bool action only when they use exactly the same checked binding.
   A rejected toggle cannot be reliably rolled back without a fresh shell-capability resync because `sdk/app.Client.Publish` suppresses unchanged snapshots. Keep `OnBool` handlers infallible after validation, or ensure their state change is successfully published. M5 will add explicit resynchronization.
3. **Static screen tree changes still cause full `View` publication.** `Client.Publish` can suppress identical full views; `Invalidate` queues a view rebuild but does not introduce fine-grained widget patching. M5 will add optional capability-negotiated widget-property patches.
4. **Bound values are validated at publication.** String limits use the field's schema cap (80, 160, 240 bytes; M6 extends screen subtitles to 1,024 bytes), invalid UTF-8 is rejected, numeric values must stay within declared ranges, and the serialized `view` v7 IPC envelope must fit `64 KiB`. No invalid view is sent to the shell.
5. **Full views preserve stable screen/widget IDs**, including deterministic synthetic group heading IDs, so the shell retains focus and navigation where possible. All advanced layout templates currently use the 0.5.0 shell's existing row layout; distinct geometric template treatment remains M5.
6. **Text editing remains an SDK app operation:** call `Client.RequestText()` in an `OnAction` handler; process unhandled `text_result` events in the application and `Invalidate` affected bindings. No secret text is sent through YAML.

## Quick verification on the Pi (no root, DRM or service interruption)

Use a checkout of the integrated repository. The supplied incremental archive targeted the older M2 snapshot; current `main` includes the adaptations to its CI and acceptance scripts, so do not overwrite those files with the original archive:

```sh
cd ~/roundos
./scripts/m3/verify.sh
```

The script checks historical provenance fixtures and all three Python suites, all Go tests including real v7 client/socket integration, `go vet`, RUI validation/previews, reproducibility of the embedded `.rui.bin`, and ARMv7 non-DRM builds. It uses temporary outputs and does not install or start anything. Main CI also runs the race detector and DRM checks. On a 32-bit Pi, Go `-race` is unsupported; the M3 script does not invoke it.

To try the developer tool and sample **without changing the installed RoundOS service**:

```sh
mkdir -p ~/bin
go build -o ~/bin/roundui ./cmd/roundui
go build -o ~/bin/round-rui-demo ./examples/rui-demo
~/bin/roundui inspect examples/rui-demo/layout.rui.bin
~/bin/roundui preview -o /tmp/demo-rui.png examples/rui-demo/layout.rui.bin
```

The demo binary requires RoundOS supervisor environment/IPC and cannot be launched directly in an ordinary SSH shell. Installing the demo is optional and requires administrative review; **do not** install arbitrary code as root or modify `/opt/roundos/current`. For a reviewed demo binary, the release supports root-owned executables and manifests under `/etc/roundos/apps` followed by a deliberate service restart; see `docs/SDK-APPS.md`. Manifest:

```json
{"id":"org.example.rui.demo","name":"RUI Demo","icon":"RUI","exec":"round-rui-demo"}
```

## Acceptance checklist

- [x] SDK runtime and opt-in demo added without replacing current shell/service changes
- [x] Runtime rechecks compiled manifest, source map, checksum, and app ID
- [x] Exact type-checked registration, rejects missing/extra/duplicate keys
- [x] All nine RUI widgets lower to existing v7 UI model
- [x] Toggle, integer commit, action, visibility, navigation, dialogs, text editor pass-through
- [x] Invalidation while publishing, retry after failure, bounds and payload checks
- [x] Real SDK client/socket integration tests, no shadow event reader
- [x] Embedded independent sample and ARMv7 non-DRM build
- [ ] Native Pi review of optional sample application interaction
- [ ] M0 outstanding physical reboot, rollback, and performance measurements

M4: dev-only hot reload and last-known-good recovery. M5: negotiated incremental updates. Neither is claimed implemented in M3.
