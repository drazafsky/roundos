# RoundOS RUI v1 — Final Implementation Plan and Technical Specification

**Status:** Approved architectural choices; implementation specification (not implemented code)  
**Baseline:** RoundOS **0.5.0**, archive SHA-256 `06a1241970b365a4d91fa5bee65561322d155cd79303c08d097bd7035e100c75`  
**Target:** Backward-compatible declarative UI platform on Raspberry Pi Zero 2 W / HyperPixel 2.1 Round (480 × 480)  
**Versioning:** `roundos.rui/v1` (layout language); remain IPC v7 for initial releases; add separately negotiated capabilities, without breaking v7 applications  
**Document date:** November 19, 2026

## 1. Goals, accepted decisions, and exclusions

### 1.1 Binding decisions

| Concern | Decision |
|---|---|
| Developer-authored syntax | **YAML**, strictly validated and versioned |
| Layout model | **Shell-managed templates and semantic groups**; no absolute positions |
| Migration scope | **Applications first**: Clock, Timer, independent demo; shell Settings/Wi-Fi later |
| Development experience | **Validated hot reload** for trusted development sessions |
| Production experience | **Compiled, embedded** layout bundles; no YAML parsing per screen frame or tick |
| Compatibility | **Existing IPC v7 applications must continue working**; incremental and capability-gated transport enhancements |

### 1.2 Goals

1. Allow an application author to describe screens, controls, navigation, accessibility, and bindings in `.rui.yaml`, with application state and business rules retained in Go.
2. Preserve the existing `sdk/app`, `sdk/ui` and `wire.View` idioms for programmatic applications; make both authoring paths compile to the **same semantic model**.
3. Keep all geometry, clipping, touch hit regions, focus, theme colors, navigation stacks, pickers, dialogs, text input, and DRM rendering in the shell.
4. Provide typed bindings, explicit action handlers, deterministic update batching, full-view fallback, optional efficient property updates, and deterministic recovery from interruption.
5. Preserve 0.5.0 app lifecycle, supervisor, root-managed install/rollback, Wi-Fi, performance collection, existing UI, and hardware gesture behavior.

### 1.3 Explicit non-goals for v1

- A new embedded programming language, arbitrary template expressions, JavaScript, shell commands, or embedded Go evaluated from YAML.
- Free-form pixel coordinates, per-app RGB colors, arbitrary flexbox/CSS, user-definable low-level drawing primitives, or complete renderer replacement.
- Migration of built-in Settings/Wi-Fi shell screens during v1; leave those implementations operational and unchanged.
- Per-app security sandboxing or distribution of untrusted apps. The 0.5.0 shared `roundos-app` UID constraint remains.
- Network downloads of layouts or run-time modification of privileged installed code.
- Mandatory recompilation or replacement of existing optional-app binaries merely to install the new shell.

## 2. Verified 0.5.0 baseline and architectural invariants

The baseline source contains:

- `internal/wire`: `Version = 7`, JSON length-prefixed frames limited to `MaxMessage = 64 << 10`; structures `View`, `Screen`, `Widget`, `Dialog`; full-view validation; scalar `ViewPatch`.
- `sdk/app`: `Connect()`, `Events()`, `Publish(ui.View)`, `RequestText(...)`, `Notify(...)`, `SubscribeContacts(...)`; `Event` exposes `Type`, `Action`, `Delta`, `Visible`, etc.
- `sdk/ui`: public aliases plus `Button`, `PrimaryButton`, `Link`, `Toggle`, `Slider`, `ListItem`, `Label`, `Progress`, `NumberPicker`, `TimePicker`, `DatePickerDays`, `ConfirmDialog`, and `Screen`.
- `internal/shell`, `internal/circleui`: retained widget rendering and hit testing, focus, per-app navigation, modal/picker behavior, and circular geometry (`SafeRadius = 218` for the 480 × 480 panel), including pixel-level scrolling.
- `internal/appmgr`: authenticated Unix socket, foreground/background app lifecycle, app registry; root-controlled optional manifests under `/etc/roundos/apps`; max 12 optional apps.
- `scripts/install-release.sh`, `roundosctl.sh`, `systemd/roundos.service`: root-owned immutable managed releases under `/opt/roundos/releases`, symlinks `current`/`previous`, explicit activation and rollback.

**Critical behavior:** SDK `Publish` currently sends a complete `View` when `Screens` or `Dialog` changes. Its existing scalar patches cannot individually update nested widget properties. **Also, `wire.Read` and `wire.Write` reject mismatched protocol versions**; a simple global increment to 8 would break old applications.

**Baseline release status:** RC4 Wi-Fi GUI hardware testing was reported successful, but the new 0.5.0 unattended systemd boot/rollback/performance acceptance remained unverified in the release notes. Resolve that gate before relying on the managed service as a production deployment baseline.

## 3. Intended architecture

```text
application source
  layouts/*.rui.yaml  ──> roundui compile ──> embedded .rui.bin
  Go logic + typed bindings + named handlers │
                                              ↓
                               sdk/rui runtime (new)
                         static tree + evaluated properties
                          │          │           │
                          │          │           └── app state invalidation
                          │          └── action dispatch from sdk/app events
                          └── sdk/app Publish / optional capability-gated patches
                                              ↓
                         authenticated IPC v7 compatibility transport
                                              ↓
               existing shell: widget tree + focus + nav + theme + DRM
```

Core principle: **YAML describes presentation and intent; Go owns data, persistence and effects; the shell owns screen geometry and interaction.**

### 3.1 Proposed repository structure

```text
cmd/roundui/                      # build-time validator/compiler/preview tool
sdk/rui/                          # public runtime, bindings, action dispatch
sdk/rui/spec/                     # public schema/types (no internal/wire imports required by consumers)
internal/rui/parse/               # strict YAML parser and source positions
internal/rui/compile/             # normalized IR, validation, deterministic output
internal/rui/runtime/             # retained binding/dependency evaluation
internal/rui/preview/             # render-state fixture conversion for existing PNG backend
internal/wire/                    # additive, capability-gated patch fields/types
internal/appmgr/                  # capability negotiation and safe patch delivery
internal/shell/                   # property patches, group/template layout support
examples/rui-timer/              # independent external SDK example
schemas/roundos-rui-v1.schema.json
layouts/clock.rui.yaml
layouts/timer.rui.yaml
scripts/tests/                     # development and regression checks
```

Names are proposed. Keep the `sdk/rui` package public and do not require external apps to import `roundos/internal/*`. The schema/toolchain may be split into another Go module later, but that is not required for v1.

## 4. RUI v1 schema

### 4.1 Document contract

Top-level keys:

| Field | Type | Requirement |
|---|---|---|
| `schema` | literal string | Must equal `roundos.rui/v1` |
| `application.id` | string | Must match the trusted application identity/manifest at runtime |
| `screens` | nonempty sequence | First screen is root; unique screen IDs |
| `screens[].id` | stable string | Matches SDK target IDs; max 64 bytes to respect existing wire validation |
| `screens[].title` | text value | Literal or typed string binding |
| `screens[].layout` | enum | `hero_list`, `list`, `form`, or `detail` |
| `screens[].hero` | text value | Optional; only available with compatible templates |
| `screens[].subtitle` | text value | Optional |
| `screens[].groups` | sequence | Semantic section groups, flattened in a controlled way when targeting v7 |
| `groups[].id` | stable string | Unique within screen |
| `groups[].title` | text value | Optional section label, nonfocusable |
| `groups[].widgets` | sequence | Supported widgets with explicit IDs and typed properties |

A **text value** is either a literal string or `{ bind: binding.name }`. An **int/bool value** is a literal of that type or `{ bind: binding.name }`. No expression interpolation, functions, operators, template evaluation, or uncontrolled string substitution.

### 4.2 Widget contract

Required `id` and `type`; optional `label`, `detail`, `variant`, `enabledWhen`, `visibleWhen`, and relevant widget-specific values and actions. Supported types map to the nine controls already in 0.5.0:

| RUI type | Existing semantic widget | Binding/event contract |
|---|---|---|
| `button` | `button` | `onPress` action OR `navigate` screen |
| `toggle` | `toggle` | `checked: {bind: bool}` + `onChange` action |
| `slider` | `slider` | `value: {bind: int}`, range/step, `onCommit` with **absolute integer** |
| `listItem` | `list_item` | Optional detail; `onPress` or `navigate` |
| `label` | `label` | Bound text; not focusable |
| `progress` | `progress` | Bound integer in validated range; not focusable |
| `numberPicker` | `picker` | Absolute integer commit, bounds and wrapping policy |
| `timePicker` | `time_picker` | Absolute minutes since midnight `0..1439` |
| `datePicker` | `date_picker` | Absolute UTC days since Unix epoch |

`variant` is restricted to `primary`, `secondary`, `tertiary`. No color, font-family, RGB, framebuffer coordinates, user-defined hitboxes, or arbitrary embedded images in v1. Confirmations use a **declarative dialog spec** mapped onto existing `View.Dialog`; text editing references the current shell `RequestText` facility.

### 4.3 Example layout

```yaml
schema: roundos.rui/v1
application:
  id: org.roundos.timer
  icon: TMR
screens:
  - id: timer.root
    title: Timer
    layout: hero_list
    hero: { bind: timer.display }
    subtitle: { bind: timer.status }
    groups:
      - id: actions
        widgets:
          - id: timer.toggle
            type: button
            variant: primary
            label: { bind: timer.toggleLabel }
            onPress: timer.toggle
          - id: timer.durationLink
            type: button
            label: Duration
            navigate: timer.duration
  - id: timer.duration
    title: Duration
    layout: form
    groups:
      - id: duration
        title: Set Duration
        widgets:
          - id: timer.minutes
            type: numberPicker
            label: Minutes
            value: { bind: timer.minutes }
            min: 1
            max: 999
            step: 1
            enabledWhen: { bind: timer.stopped }
            onCommit: timer.setMinutes
          - id: timer.reset
            type: button
            label: Reset to Five
            onPress: timer.reset
```

**Compiler mapping:** `groups` and `layout` map to shell-owned structures. The initial compatibility renderer can flatten simple groups to existing nonfocusable headings and widgets; more advanced visual group semantics require a matching shell capability. All generated widget IDs must remain stable. The compiler must report rather than silently drop unsupported constructs.

### 4.4 Constraints and semantics

- Maximum 16 screens and 64 effective widgets per screen, consistent with `wire.ValidateView`, unless a future explicitly versioned capability raises those limits. Count compiler-added section headings and generated nodes in the effective widget budget.
- Full serialized v7 `View` must be **under 64 KiB**; compilation and runtime publication must check actual encoded frame size, not merely element counts.
- Screen and widget identifiers are case-sensitive, stable and unique in their scopes. A change of ID is a removal/addition and may reset focus. `navigate` must resolve to a screen in the same application; cycles are permitted for explicit user navigation only with a bounded stack depth.
- `visibleWhen`/`enabledWhen` require **bool** bindings. Hidden controls must not receive focus or pointer events. When a focused control becomes hidden/disabled, move focus deterministically to the nearest available control, or Back if none exists.
- No arbitrary field reflection or dynamic evaluation; all binding keys and action names must be declared and checked at startup. RUI source cannot gain app privileges or bypass manifest permissions.
- Repeating content, if implemented in v1, must be a bounded `repeat` with stable item keys and a maximum expansion count. No open-ended recursive component expansion.
- YAML parsing must reject duplicate mappings, unknown fields (except explicitly defined extensions), excessive nesting/aliases, ambiguous scalar coercion, and unsafe file includes. `schema` version must be exact.

## 5. New public SDK APIs

### 5.1 Package goals

Add `roundos/sdk/rui` **without changing** the public `sdk/app` contract. A RUI runtime may share the existing `client.Events()` stream with legacy code; it must **never start a competing event reader**. It must permit normal app notifications, background goroutines, IPC text editor requests, and legacy handlers.

Illustrative intended API (final Go signatures and types to be pinned in Milestone M1):

```go
//go:embed layouts/timer.rui.bin
var layout []byte

client, err := app.Connect()
if err != nil { return err }
uiRuntime, err := rui.New(client, layout)
if err != nil { return err }

uiRuntime.BindString("timer.display", func() string { return timer.Display() })
uiRuntime.BindString("timer.status", func() string { return timer.Status() })
uiRuntime.BindString("timer.toggleLabel", func() string { return timer.ActionLabel() })
uiRuntime.BindInt("timer.minutes", func() int { return timer.Minutes() })
uiRuntime.BindBool("timer.stopped", func() bool { return !timer.Running() })

uiRuntime.OnAction("timer.toggle", func(ctx context.Context, e rui.ActionEvent) error {
    timer.Toggle()
    uiRuntime.Invalidate("timer.display", "timer.status", "timer.toggleLabel", "timer.stopped")
    return nil
})
uiRuntime.OnInt("timer.setMinutes", func(ctx context.Context, value int) error {
    timer.SetMinutes(value)
    uiRuntime.Invalidate("timer.minutes", "timer.display")
    return nil
})

if err := uiRuntime.ValidateBindings(); err != nil { return err }
for {
    select {
    case event, ok := <-client.Events():
        if !ok { return nil }
        handled, err := uiRuntime.Dispatch(event)
        if err != nil { log.Print(err) }
        if !handled { handleExistingAppEvent(event) }
    case <-timerTick.C:
        uiRuntime.Invalidate("timer.display")
    }
    if err := uiRuntime.Flush(); err != nil { return err }
}
```

**Threading semantics:** callbacks execute on the application's event loop; `Invalidate(...)` may be called by worker goroutines and only queues a changed key. Bound getters must read a consistent application state snapshot. `Flush()` evaluates and publishes on the designated application event loop (or a single serialized SDK loop in the convenience `Run()` variant). No callback executes on the shell render thread. A useful convenience `Run(ctx)` API may be added after the explicit event-loop API is correct.

### 5.2 Binding system

Proposed registered value types:

- `BindString(key, func() string)`
- `BindInt(key, func() int)`
- `BindBool(key, func() bool)`
- `BindTextList(key, func() []rui.Item)` for bounded stable-key lists (defer until controlled repeat is ready)
- `OnAction(key, func(context.Context, rui.ActionEvent) error)`
- `OnInt(key, func(context.Context, int) error)`
- `OnBool(key, func(context.Context, bool) error)` (true/false exact-state contract in upgraded UI-event capability)
- `Invalidate(keys ...string)`, `ValidateBindings()`, `Dispatch(app.Event)`, `Flush()`

Compile-time validation checks schema shape and value categories; runtime `ValidateBindings` rejects missing/extra-incompatible registrations and dangerous event/value combinations before the first `Publish`. A generated constants/registration manifest can tighten compile-time ergonomics without making code generation mandatory.

**Important v7 behavior:** existing `input` events contain `Action` and integer `Delta`, not the explicit toggle's checked value. The v7 fallback adapter implements toggles using the application's current state and republishes the authoritative value; it must not claim to offer idempotent exact-value events. In an upgraded negotiated UI-event capability, `OnBool` can receive the explicit target state along with a unique event ID and revision.

### 5.3 Reactive evaluation

- Generate a dependency map from binding keys to affected widget properties during compilation; store it in the bundle, not as runtime string parsing.
- `Invalidate` marks dirty bindings; batch updates on the app's serial event loop. Coalesce redundant invalidations. Do not publish if semantic values are unchanged.
- Update static structure only when its shape changes (conditional visibility, repeated items, navigation targets); update properties separately when supported.
- Preserve focus, scroll, draft state, and navigation stack for stable screen/widget identities. Never reset the active screen simply because a bound label changed.
- Rate-limit high-frequency values, optionally using per-binding scheduling policies such as `onChange` or a bounded maximum refresh rate; do not create a 60-FPS republish loop for a 1-Hz timer.
- Invalid values or failed getters: log a sanitized structured error and retain the last valid property where safe. Never panic the DRM shell for an app-side binding error.

## 6. Compilation, hot reload, and production resources

### 6.1 CLI design

```sh
roundui validate layouts/timer.rui.yaml
roundui compile layouts/timer.rui.yaml --output layouts/timer.rui.bin
roundui inspect layouts/timer.rui.bin
roundui preview layouts/timer.rui.yaml --screen timer.root --state tests/fixtures/timer.json --theme dark --output /tmp/timer.png
roundui dev layouts/timer.rui.yaml --app ./round-timer
```

`roundui` diagnostics include file, line, column, screen/widget ID and actionable expected/actual type text. `validate` must be usable in CI without device access. `preview` should use the **same shell layout/hit-test and drawing code** as production, not a visually unrelated mock renderer. Provide fixture state for bindings and deterministic time/locale. `inspect` reports schema version, compiled fingerprint, screens, bindings, handlers and budget usage.

### 6.2 Bundle format

Use a small deterministic, versioned binary envelope containing canonical, normalized JSON (or equivalently compact deterministic encoding), a schema/format version and SHA-256 content fingerprint. Avoid building a novel executable bytecode VM. The bundle contains static screen/widget definitions, typed binding references, action names, dependency indices and source-map locations for diagnostics. It contains **no credentials, executable scripts or runtime business state**.

The production application embeds it via Go `//go:embed`, and the SDK parses the compiled artifact once at startup. It must not parse YAML every time a screen opens or a clock ticks. Rebuilding an app is allowed to update its embedded layout; changing themes does not require recompilation.

### 6.3 Hot reload design

- **Development only**, explicitly enabled by developer workflow, using a trusted application-owned workspace path. No hot reload from arbitrary folders in the root-run production service.
- Watch/poll with debounce, compile+validate into a **staging tree**, and atomically swap only on success. On error, keep last-known-good UI and show diagnostics in the dev tool, not on the physical production display.
- Preserve current screen and focus for stable IDs; fall back to the root screen/nearest focusable control when a node was deleted. Preserve application **state and Go handlers** through a layout-only reload; never hot-reload the Go executable implicitly.
- No side-effect execution or action dispatch just because the YAML file changed. Cancel pending gestures/slider transactions cleanly before swapping incompatible layouts.
- Secure text editor contents must never be emitted to live-reload logs, preview state fixtures or bundle contents.

## 7. IPC v7 compatibility and optional enhancements

### 7.1 Phase A: no wire change

`RUI -> ui.View -> sdk/app.Client.Publish(...)` using the existing v7 protocol. Current validation and `64 KiB` guard remain unchanged. This is sufficient to implement YAML compilation, named Go actions, and reactive state bindings functionally, though nested widget changes still cause full-view publication.

### 7.2 Phase B: negotiated additive v7 capability

Extend the **upgraded** shell-to-app initial `visibility` message with optional feature information (for example `capabilities: ["widget_patch_v1", "ui_event_v1"]`) while keeping `Version: 7`. Old v7 clients ignore unknown JSON fields. Updated SDK clients assume **no capabilities** if the field is absent, and continue sending ordinary full views to an unmodified 0.5.0 shell.

New optional message types (only sent after an explicit capability advertisement):

```json
{
  "version": 7,
  "type": "widget_patch",
  "layout_revision": 4,
  "base_revision": 18,
  "new_revision": 19,
  "updates": [
    {"screen": "timer.root", "widget": "timer.toggle", "property": "label", "string": "Pause"},
    {"screen": "timer.root", "widget": "timer.progress", "property": "value", "int": 275}
  ]
}
```

Additional optional `ui_event_v1` messages can carry typed widget input (`screen`, `widget`, `action`, `kind`, `bool` or `int` value, originating `layout_revision`, unique monotonic event ID). **This does not replace legacy `input` events for existing apps.** The shell never sends new events to peers that have not negotiated them.

### 7.3 Correctness rules

- Maintain a **per-connection** layout revision and property revision; a full view establishes a new authoritative snapshot. Property updates apply only when their base revision matches the shell's retained state and all IDs/types validate.
- Apply a batch **atomically** after validation. On mismatch, unknown target, forbidden property, oversize batch, stale revision or app reconnect, request a full snapshot (`resync_required`) rather than applying a partial update. Never leave focus or layout in a half-updated state.
- Permit only a strict whitelist of mutable properties: text, detail, value, checked, disabled, and visibility (where shell supports its focus transitions). Layout, widget type, action target and navigation changes require a full view/tree revision.
- Coalesce redundant app updates and limit sender queues. Preserve the 64 KiB frame bound and backpressure protections. Patch batches that would exceed the bound must be divided safely or converted to a validated full snapshot if that snapshot itself fits.
- Existing v7 app binaries retain their exact `hello`, `view`, `view_patch`, `input`, `text_result`, notification and lifecycle behavior; no forced SDK rebuild or ABI changes. Shell changes require cross-version integration tests with a separately compiled **unmodified 0.5.0 app binary**.
- Do **not** globally increment `wire.Version` to 8 as part of the first RUI release. If a future v8 is needed, create a version-aware read/write adapter and explicit negotiation/migration; do not assume the current strict v7 reader supports it.

## 8. Layout templates, navigation, and accessibility

### 8.1 Supported v1 templates

| Template | Role | Required shell behavior |
|---|---|---|
| `hero_list` | Clock/Timer primary value plus actions | Header/hero, bounded focusable controls, circle-safe rows |
| `list` | Settings and navigation | Section labels, inertial touch scroll, rotary highlight, focus restoration |
| `form` | Number/date/time/settings controls | Explicit value commit/cancel semantics and large target support |
| `detail` | Read-only information plus actions | Long text clipping/truncation and Back affordance |

Groups are **semantic sections**, not arbitrarily nested containers. For v1, allow one level of section groups within a screen. Nested screens represent navigation. Compound templates can be introduced in a separately versioned schema and shell capability.

### 8.2 Interaction ownership

- Use existing central navigation stack: app-supplied `navigate` transitions to a named screen, retains parent focus/scroll, respects visible Back and rotary Back navigation, and restores state on return.
- Touch and rotary operate the same focus model. Touch hit regions and render geometry must derive from the same `circleui` layout calculations; never let YAML define its own touch rectangles.
- Confirm destructive actions using the existing shell-owned modal dialog, with safe cancellation defaults. First Back during text editing hides keyboard/preserves draft; subsequent Back honors discard confirmation.
- System themes (dark, light, high contrast), Reduce Motion, focus outline, and large touch targets apply automatically. RUI must not be able to override the global palette.
- Preserve two-contact hardware behavior (including the observed staggered-finger vertical shortcut) and correct unmirrored Y mapping; RUI must not change global touchscreen coordinate decoding.
- List and picker updates must not unexpectedly jump scroll position. When a focused widget disappears, restore to a consistent nearby enabled target or Back.

## 9. Security and failure containment

- Parse untrusted YAML only in the developer compiler; limit file bytes, alias/nesting depth, string lengths and expansion counts. No arbitrary imports or external-file includes in v1.
- Never execute YAML strings. Handler names resolve exclusively through an application-registered Go action table. Errors and unknown actions are rejected with diagnostics before initial publication.
- Preserve authenticated Unix IPC with peer UID/token validation. RUI cannot call NetworkManager, read evdev, draw directly, or acquire root privileges through new APIs.
- Preserve root-owned reviewed app manifests and executable permissions under `/etc/roundos/apps`. Do not treat a compiled declarative layout as a trust boundary: all optional apps still share the `roundos-app` UID in 0.5.0.
- Never write secure-field values to RUI bundles, logs, crash dumps, preview snapshots or general binding trace output. Text `secure` request policy retains the 0.5.0 shell's clearing/masking semantics.
- The renderer must reject invalid patch payloads and retain its last known-good tree. App crash/restart must not crash the shell; reconnected apps always begin with a complete view snapshot.

## 10. Implementation milestones, dependencies and exit criteria

| Milestone | Scope and deliverables | Acceptance gate |
|---|---|---|
| **M0: Baseline** | Branch from byte-verified 0.5.0; run existing unit/race/vet/DRM and managed-service tests; collect three idle and three interactive 60-second performance samples per scenario | 0.5.0 managed boot/rollback verified on Pi; reproducible tests and measured baseline stored; no changes to established UI |
| **M1: RUI language/schema** | RUI v1 RFC, YAML grammar, typed AST, source maps, template/group definitions, widget-to-wire mapping, schema fixtures | Valid/invalid golden tests; complete nine-widget coverage and deterministic validation diagnostics |
| **M2: Compiler/tooling** | `roundui validate`, `compile`, `inspect`, `preview`; deterministic embedded bundle; CI/lint integration | Same layout compiles to same hash; preview uses real renderer; encoded view budgets enforced; invalid source fails safely |
| **M3: SDK binding runtime** | `sdk/rui`, typed binding/action registration, dispatcher, invalidation and batching, pure v7 full-view adapter | Independent app can run all nine widgets, navigation, text/dialog hooks from YAML; zero new wire fields required |
| **M4: Development hot reload** | Explicit trusted dev mode, debounced reload, staged validate+atomic swap, state/focus preservation | Valid edit updates visible UI without app restart; invalid edit preserves last-good screen and app state; no production watcher |
| **M5: Optimized reactive updates** | Optional v7 feature advert, `widget_patch`, revision/ack/resync, optional typed `ui_event`, old-v7 fallback | Unmodified 0.5.0 app binary works; new app falls back against 0.5.0 shell; no stale/out-of-order patch corruption; no unnecessary full views on Timer tick |
| **M6: App migration and API polish** | Clock and Timer YAML layouts, tests for existing Go logic and background behavior, sample external app, SDK docs, migration guide | Behavioral/visual parity; Clock label and Timer picker/notification/crash-recovery pass; independent developer can build app from documented SDK |
| **M7: Pi validation and release** | Performance/hardware acceptance, install/rollback validation, signed-off docs, versioned release candidate | Zero functional regression vs 0.5.0; measurable latency/memory/update budgets met; irreversible state migrations absent; rollback restored from Pi test |

**Recommended implementation order:** M0 → M1 → M2 → M3 → M4 → M5 → M6 → M7. M4 and M5 may be developed in parallel once M3 owns state serialization, but release gates remain sequential. M6 can start on M3's full-view adapter to surface layout ergonomics before optimized patch transport is complete.

## 11. Testing matrix

### 11.1 Unit/schema/compiler

- YAML malformed syntax, duplicate keys, unknown fields, ambiguous booleans/numbers, deep/recursive aliases, excessive nodes, invalid UTF-8, invalid schema version, missing targets, duplicate IDs, unsupported variants.
- Each of the nine widgets, typed binding misuse, missing handler registrations, range/picker bounds, date UTC-day representation, time minute-of-day conversion, cancel/commit behavior.
- Deterministic compile hashes; source-map line/column errors; lossless compiled-load semantics and no external filesystem reads at runtime.

### 11.2 SDK/runtime

- State invalidation coalesces correctly; unchanged bindings publish no update; concurrent worker invalidations are race-free.
- App action callbacks dispatch once; disabled/hidden widgets cannot invoke handlers; v7 toggles use current-state semantics while upgraded typed events receive exact state.
- Reconnect sends authoritative snapshot; out-of-order/stale/duplicate patches trigger resync; full structure updates preserve valid focus and navigation IDs.
- Invalid binding getter does not crash shell, logs sensitive-data-safe error and retains last-good value.

### 11.3 Rendering, accessibility and input

- Golden image previews: 480 × 480 dark, light and high-contrast layouts for every template, at 100/125/150% text scales, with Reduce Motion and large targets.
- Render geometry matches touch hit testing, including partial rows near round edges. Oversized labels truncate safely, scroll positions remain valid and rotary navigation is complete.
- Screen navigation Back, modal cancellation, text-editor draft preservation, picker edits, confirmation, notifications and background apps behave identically to 0.5.0.
- Physical HyperPixel test: unmirrored Y taps; top-edge notifications and left-edge Back; staggered two-finger horizontal switching and upward Home; real encoder if installed.

### 11.4 Compatibility/security/deployment

- **Upgrade shell, retain unmodified v7 app binary**: connects, publishes and navigates correctly.
- **Updated app versus unmodified v7 shell**: capability absence triggers full-view fallback; no patch message sent to old shell.
- IPC oversized/invalid message, poisoned action names and malformed patch values fail safely. Shell never evaluates YAML or accepts arbitrary color/coordinate definitions.
- Validate root-owned release hash/install, explicit activation, bounded restarts, rollback to managed 0.5.0, preserved settings and Timer data, NetworkManager reconnect and LightDM restoration.

### 11.5 Proposed measurable release budgets

Do not invent baseline measurements; collect and retain them in M0. Proposed **relative** acceptance thresholds (to approve or recalibrate once actual Pi samples exist):

| Metric | Proposed gate |
|---|---|
| Input-to-present p95 for comparable gestures/controls | No more than **10% worse** than measured 0.5.0 in three same-condition runs; report absolute milliseconds |
| Idle shell CPU and RSS | No more than **10% CPU regression** and **8 MiB additional shell RSS** versus baseline, under matched conditions |
| Static-layout Timer 1-Hz ticks | After initial layout, **zero complete tree retransmits** during ordinary tick updates when `widget_patch_v1` is negotiated |
| Protocol traffic | Record baseline bytes/update; target at least **50% reduction** on Timer tick updates with negotiated patches |
| Development hot reload | Valid changes become visible within **500 ms** of a stable file save on the reference Pi, as a proposed target rather than a pre-established guarantee |
| Functional regression | **Zero** failed existing Pi acceptance tests; zero known data loss, unauthorized operations, deadlocks or unbounded updater queues |

Maintain a repeatable measurement script with timestamps, test setup, sample counts and raw results. Frame/input-to-present timing is **not** electrical touch IRQ-to-visible-pixel latency. Also collect power/thermal behavior where instrumentation is available; avoid claiming validated improvements solely from PNG or host tests.

## 12. Versioning, compatibility, installation and rollback

- `roundos.rui/v1` describes the **file format**, independent of RoundOS's release number or IPC version.
- Initial source branch: `feature/rui-v1` from exact 0.5.0. Milestone tags can use `rui-m1`…`rui-m7`; propose next feature release **0.6.0**, subject to the project's versioning decision. No need to claim 0.5.0 itself supports RUI YAML.
- Keep `roundosctl` install/verify/activate/rollback and root-managed immutable releases. Incorporate the compiler into developer CI/build tooling, not the privileged runtime shell dependency graph.
- Old app binaries and existing config/data formats remain usable. Never overwrite third-party app manifests, NetworkManager profiles or Timer state during RUI deployment.
- On incompatibility, installer/runtime rejects new features safely and provides a documented rollback or full-view fallback. No irreversible persisted-data migrations in RUI v1.
- Build and test an **independent Go module** consuming only `roundos/sdk/app`, `roundos/sdk/ui` and `roundos/sdk/rui`, using the 0.5.0 external-module pattern until a stable module distribution is established.

## 13. Codex-oriented implementation instructions

1. Start from the archive with the exact SHA-256 listed above. Inspect `docs/RELEASE-0.5.0.md`, `docs/SDK-APPS.md`, `internal/wire/*`, `sdk/{app,ui}`, `internal/shell`, `internal/circleui`, `internal/appmgr`, and the managed release scripts before editing.
2. Make **M0** independently verifiable; never assume previous RC4 graphical acceptance automatically validates the 0.5.0 systemd lifecycle.
3. Implement a single milestone per reviewable change set. Include tests and schema documentation with each feature; never alter the DRM/touch driver to compensate for a RUI layout issue.
4. First ship RUI-to-v7 `View` compilation and Go bindings, **before** protocol changes. Validate independent external SDK compilation.
5. Never relax the 64 KiB, screen/widget-count, identity, manifest, privilege or authentication contracts silently. A separate capability/explicit schema revision must authorize each change.
6. Keep compilation pure and deterministic, render changes on the shell-owned UI path, and execute application handlers only in the application event loop.
7. Preserve existing apps and legacy UI methods. New patch messages may only be sent after explicit peer feature negotiation. Test rollback using a binary compiled **before** RUI changes.
8. Use current `go test ./...`, `go test -race ./...`, `go vet ./...`; add golden, fuzz/property, process-integration and Pi hardware regression tests for the new functionality.
9. At each milestone deliver: source change, tests, CLI/SDK docs, sample YAML/Go, release notes, any migration steps, and a checkable acceptance checklist. Do not mark Pi hardware gates passed until the user reports the results.

## 14. Final definition of done

RUI v1 is complete only when an independently authored app can define its screens and all supported controls **entirely in YAML**, embed the compiled bundle, register typed Go state/actions, and operate with the same UI/rotary/touch/accessibility semantics as native RoundOS applications. Development hot reload must safely preserve last-good UI, optimized property updates must work only when negotiated, unmodified v7 apps must remain functional, and the managed Pi release must pass real hardware tests with verified rollback and no regression against the 0.5.0 baseline.
