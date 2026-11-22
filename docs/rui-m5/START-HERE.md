# M5: negotiated reactive UI

RUI language v1 and IPC v7 remain supported. New SDK clients offer capabilities
in `hello`; the manager selects their intersection in the initial `visibility`
message. An absent selection retains the original v7 full-view/scalar-patch
transport. The SDK preserves `Event.Type == "input"` for existing app loops;
typed events additionally expose `Event.UI`.

- `widget_patch_v1`: revisioned full snapshots, typed property updates,
  acknowledgements and authoritative resync.
- `ui_event_v1`: screen/widget origins, exact bool/int values, a layout revision
  and a monotonic event ID. RUI deduplicates these IDs and rejects stale,
  hidden, disabled or incompatible targets. The legacy bool adapter continues
  to use the current Go state because original input has no exact bool value.
- `rui_templates_v1`: shell-owned `hero_list`, `list`, `form` and `detail`
  presentation plus semantic section identities. Without this selection the
  SDK sends the documented row-list fallback, retaining headings, IDs, actions
  and navigation.

## Bindings and bundles

The compiler now writes `RUI2` / `roundos.rui.bundle/v2` bundles containing
verified dependency indices. Existing `RUI1` bundles still load; their indices
are derived once at startup. The layout language remains `roundos.rui/v1`.
Checksums cover the canonical payload; loading re-derives and verifies the
manifest, dependency indices and source map. Bundles contain no executable
callbacks, application state or credentials.

Call `Invalidate("key", ...)` after changing those bindings, then `Flush()` on
the application's serial event loop. Repeated invalidations coalesce. Getters
run once for each dirty key; affected screens are rebuilt and unchanged semantic
views are skipped. `Invalidate()` without keys reevaluates everything. Handler
dispatch invalidates all bindings for compatibility with existing callbacks.
Visibility restoration also reevaluates state. Getters must read a consistent
application-owned snapshot.

Invalid values or panicking getters return a sanitized `BindingError`. A
previously valid property is retained while other valid bindings update; a
failed first evaluation prevents publication. Failed bindings remain dirty for
retry. Error messages identify the binding and reason without its value or panic
payload. Nothing evaluates YAML or business callbacks on the render thread.

`roundui inspect -json app.rui.bin` includes the dependency graph. Recompile
embedded resources with the current compiler before rebuilding an application.
The checked-in independent demo includes the current deterministic bundle.

## Revisions, recovery and bounds

Each connection starts without a retained modern snapshot. A versioned `view`
establishes a new `layout_revision` and `new_revision: 1`. A `widget_patch`
requires that layout and the exact current `base_revision`, then advances to
`new_revision == base_revision + 1`. The manager validates the whole batch on a
private copy before reporting a view and sending `view_ack`. A stale revision,
unknown target, invalid type or forbidden property yields `resync_required`
without changing the retained view. Reconnection requires a full snapshot.

Mutable fields are bounded view/screen text, widget label/detail/value/checked/
disabled, and dialog text. Dialog updates identify their target with `dialog`;
screen/widget updates use `screen` and `widget`. IDs, groups, templates, ranges,
actions, navigation and dialog structure require full snapshots. Conditional
visibility also changes structure. Every frame remains at most 64 KiB, with
the original 16-screen/64-widget budgets and a 2,048-property batch bound.

The SDK tracks one pending update, the latest desired view and at most one dialog
clear barrier. These are bounded snapshots,
not an accumulating event queue. A clear/reopen sequence survives coalescing and
resync. Writes have a two-second deadline and an unacknowledged update closes
the connection after three seconds. Ordinary application notifications and
editor events keep using the same reader and bounded event stream.

`PublishAuthoritative` restores unchanged state after rejected optimistic input.
`PublishSnapshot` synchronously writes a complete replacement before development
reload commits new action metadata. It supersedes an older pending revision and
ignores its late acknowledgement, without waiting on the app loop's event
consumer. A failed write closes that connection. A control acknowledgement does not prove a physical display frame
has appeared; the physical reload budget is still an M7 measurement.

## Templates and input

Production drawing, preview, rotary focus, touch hit-testing and kinetic
scrolling share `circleui` metrics. Hero and list screens retain established
geometry; forms reserve taller edit controls, and detail screens wrap and
truncate read-only text within circle-safe rows. Sections are nonfocusable
headings. Pickers/sliders retain control-level commit and cancel behavior;
forms do not invent a transaction covering the whole screen.

The shell records the actual control that produced each typed action rather
than guessing its identity from an action name. Inputs carry the revision of
that model. New structural/action changes cancel held contacts through release
and preserve compatible editing drafts. Dialog and notification actions retain
their existing input route.

## Verification

Run `sh scripts/m5/verify.sh`. Main CI also runs the race suite and DRM/ARMv7
builds. Tests cover negotiation, bounded coalescing, acknowledgement timeout,
resync/replay/atomic rejection, dialog barriers, exact-value input, duplicate
IDs, disabled/hidden guards, dirty evaluation and last-good recovery, template
geometry, and source-verified compatibility in both directions. The immutable
0.5.0 fixture is described in
[`tests/integration/testdata/README.md`](../../tests/integration/testdata/README.md).

Clock/Timer migration is M6. Actual Pi performance, display/touch/encoder,
managed installation, rollback and stable 0.6.0 acceptance remain M7 gates.
