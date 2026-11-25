# RUI v1 — Frozen M1 authoring-language contract

**Status:** M1 syntax and semantic validation implemented. No production RUI
runtime/compiler or shell behavior is enabled by this milestone.

Baseline is the byte-verified RoundOS **0.5.0** release, IPC **v7**. None of
its 149 original source files have been changed. M1 has no network dependency
and adds only build-time/test packages, a public typed syntax model, a JSON
schema, example layouts and fixtures. The installed application, service,
DRM/touch/rotary, NetworkManager, boot configuration and managed release
remain unchanged.

## Selected product decisions

- Developer-facing format: **YAML**, `schema: roundos.rui/v1`.
- Shell-managed `hero_list`, `list`, `form`, `detail` templates; one level of
  semantic groups inside a screen. No absolute sizes, positions, colors or
  arbitrary renderer operations.
- Application state and handlers remain in Go. YAML action names and binding
  names are inert references and never contain executable code.
- M2 will compile source into a production embedded bundle; M4 will add a
  trusted development hot-reload path. M1 does **not** implement either.
- Existing IPC v7 and `sdk/ui` programs continue without modification.
  M3 will initially adapt RUI to ordinary `wire.View` publication.

## Exact YAML authoring subset accepted by M1

The M1 reader intentionally recognizes a bounded subset of YAML 1.2:

- UTF-8 text, at most 128 KiB / 2,500 lines / nesting depth 16.
- Mapping entries `key: value` and indentation-based sequences `- key: value`.
- Two spaces per indentation level; no tabs.
- Unquoted plain strings, decimal integer scalars, lowercase `true`/`false`,
  single quoted strings (with doubled apostrophes), and JSON-style double
  quoted strings (with backslash escapes).
- A single-key inline mapping **only** for typed bindings:
  `{ bind: timer.display }`. The equivalent block form is also accepted.
- Line comments introduced by a `#` preceded by whitespace, outside quotes.
- Unquoted `null`, `yes`, `no`, `on`, `off`, NaN, infinity and numeric strings
  in string positions are rejected to avoid YAML scalar coercion surprises.
- Other YAML features are intentionally **unsupported and fail closed**:
  aliases, anchors, merge keys, tags, directives, multiple documents, flow
  sequences, flow mappings other than `bind`, block literals (`|` or `>`),
  arbitrary includes, anchors, and templates/expressions.

A general YAML parser may be adopted during M2 only if these restrictions and
identical validation/error behavior are preserved. This deliberately strict
subset does not claim to accept every valid generic YAML file. Quoting a
numeric-looking label is required; literal dates should be explicit **UTC
integer epoch days**, not unquoted date strings.

## Top-level structure

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
    groups:
      - id: actions
        widgets:
          - id: timer.toggle
            type: button
            variant: primary
            label: { bind: timer.toggleLabel }
            onPress: timer.toggle
```

- `application.id`: trusted application identity; the future runtime must
  compare it to authenticated app identity/installed manifest.
- `application.icon`: required static icon token of 1–3 uppercase ASCII letters
  or digits, matching the format used in trusted installed-app manifests
  (for example `CLK`, `TMR`, or `RUI`). Bindings, paths and embedded images are
  not accepted. Declarations without an icon fail validation, including
  programmatically constructed documents. Add the icon and recompile older
  layouts and bundles with the current tooling. The installed manifest remains
  the launcher's trusted icon source; built-in apps use their themed symbols.
- `screens`: 1 to 16 in declared order; **first** screen is root.
- Each screen: stable `id` (up to 64 ASCII bytes), `title` (string or string
  binding), template `layout`, optional `subtitle`, and one or more `groups`.
  `hero` is required for `hero_list` and forbidden in other templates.
- `groups`: one semantic level, `id`, optional `title` and nonempty `widgets`.
  A group heading costs an effective widget slot in the v7 fallback.
- IDs and action/binding names: ASCII `[A-Za-z][A-Za-z0-9_.-]*`, 1–64 bytes.
  IDs beginning `rui.` or `__rui_` are reserved for the compiler/shell.
  Screen IDs are document-unique; group IDs and widget IDs are unique within
  their screen. No automatic ID generation for user widgets.
- Optional `dialogs`: predeclared shell-owned confirmations with `id`, `title`,
  optional `body`, `confirmLabel`, `cancelLabel`, `onConfirm`, `onCancel`, and
  optional literal boolean `destructive`. Dialogs do not execute or open by
  themselves; M3 app handlers trigger existing shell dialogs.

### Value syntax and types

The only value forms are a typed literal or a registered binding reference:

```yaml
label: Start
subtitle: { bind: timer.status }
value: 15
value: { bind: timer.minutes }
checked: true
checked: { bind: settings.soundEnabled }
enabledWhen: { bind: timer.stopped }
visibleWhen: { bind: timer.showAdvanced }
```

`enabledWhen` and `visibleWhen` **must be boolean bindings**, not literal
booleans. All binding keys are statically registered in an M1 `Manifest` as
`string`, `int`, or `bool`. The validator rejects reuse of one binding key with
incompatible types. Missing Go handlers/bindings cannot be checked until M3.

### Templates

| Name | Intended shell semantics |
|---|---|
| `hero_list` | Large primary/hero value with a bounded list of controls |
| `list` | Focus-based rotary list, scrollable with touch |
| `form` | Editing controls; committing/canceling has explicit semantics |
| `detail` | Read-only detail/text with optional actions and Back |

These are **semantic declarations**, not pixel layouts. M3/M5 must either
render them faithfully or reject unsupported capabilities. M1 itself does not
alter current shell layout selection.

### The nine wire-compatible widget types

| RUI type | IPC v7 widget kind | Required properties | Event |
|---|---|---|---|
| `button` | `button` | `label`, exactly one of `onPress`/`navigate` | action or navigation |
| `listItem` | `list_item` | `label`, exactly one of `onPress`/`navigate` | action or navigation |
| `toggle` | `toggle` | `label`, `checked`, `onChange` | bool handler; **v7 fallback** resolves target from authoritative Go state |
| `slider` | `slider` | `label`, `value`, `min`, `max`, `onCommit` | absolute `int` |
| `numberPicker` | `picker` | `label`, `value`, `min`, `max`, `onCommit` | absolute `int` |
| `timePicker` | `time_picker` | `label`, `value`, `onCommit`; optional `step` dividing 1440 | minute-of-day integer, 0..1439 |
| `datePicker` | `date_picker` | `label`, `value`, `min`, `max`, `onCommit` | UTC epoch-day integer |
| `label` | `label` | `label` | no action/focus |
| `progress` | `progress` | `label`, `value`, `min`, `max` | read-only |

Optional semantic `variant`: `primary`, `secondary`, `tertiary` only. Optional
`detail`: text or string binding. Optional `step`: positive integer. Optional
`wrap`: literal boolean. No custom hex colors, inline styles, hit rectangles,
font families, HTML, scripts, expressions, arbitrary user-provided widgets or
unbounded repeaters.

Bound integer values must be runtime-checked within declared limits in M3;
literal integers are checked immediately at M1. All integer literals are
signed **32-bit** values and use canonical decimal syntax. Runtime action
names are inert strings until a Go callback is explicitly registered.

### UI limits and navigation semantics

- At most **64 effective widgets per screen**, counting group headings.
- Transport **64 KiB** JSON-framed v7 view limit remains unchanged. M1 source
  validation cannot guarantee that *future dynamic binding values* fit the
  limit; M2/M3 must measure the actual encoded view before publication.
- Navigation targets must reference a screen in the same document. Cycles are
  statically allowed but the existing shell's bounded navigation-stack limit
  remains in effect.
- Hidden/disabled controls must not be focusable when the later runtime applies
  these properties. Stable widget IDs preserve focus/scroll/draft state.
- Confirmations use existing shell-owned dialogs. Text entry continues to use
  existing `sdk/app.RequestText`; embedding input execution in YAML is forbidden.
- IPC v7 `input` has an action string/integer delta, *not* a typed boolean
  target. The M3 fallback must derive toggle state from application state;
  idempotent exact-value events require a later negotiated capability.

## M1 packages and API contracts

- `sdk/rui/spec`: **public** Go types `Document`, `Screen`, `Group`, `Widget`,
  `Dialog`, `Value[T]`, `Pos`, `Manifest` and `Action`.
- `internal/rui/parse.Parse([]byte) (*spec.Document, error)`: bounded pure
  parser; parse errors include one-based source line/column. No network, file
  access or arbitrary YAML types.
- `internal/rui/validate.Validate(*spec.Document) (*spec.Manifest, error)`:
  deterministic structural/semantic validation, typed binding/action manifest,
  navigation checks and existing wire limits; no application runtime.
- `schemas/roundos-rui-v1.schema.json`: JSON Schema 2020-12 **structural**
  representation. Cross-reference/uniqueness/type conflicts are enforced by
  Go semantic validation, not JSON Schema alone.
- `internal/rui/testdata/valid` and `invalid`: golden fixtures for all nine
  types, dialogs, source locations and invalid inputs.

The M1 Go parser and validator are `internal` packages on purpose: the M2
compiler will use them inside the RoundOS repository. An independently built
application imports **only** `roundos/sdk/rui/spec` and, in M3, `roundos/sdk/rui`.

## M2/M3 handoff — explicit work not yet implemented

1. `roundui validate/compile/inspect/preview`; deterministic compiled bundle,
   source maps, lint output and generated constants.
2. Verified mapping from semantic groups/templates to the existing v7
   `wire.View` and actual shell widget geometry (no silent loss of groups).
3. SDK `BindString`/`BindInt`/`BindBool`, `OnAction`/`OnInt`/`OnBool`,
   `Dispatch`, `Invalidate`, `Flush`, `ValidateBindings`, v7 full-view adapter.
4. Application identity check before publication; correct absolute-value
   slider/picker commits, v7 toggle fallback, modal/text-editor events.
5. Actual JSON wire-frame byte count validation with **dynamic** bindings.
6. Runtime invalidated/removed widget focus handling and secure text policy.
7. Dev hot reload (M4); negotiated property patches (M5) are not part of M1.

## Test instructions

```sh
cd ~/roundos-0.5.0-rui-m1
./scripts/m1/verify.sh
# Or entire source suite:
go test ./...
go vet ./...
```

Native DRM build and physical input behavior should remain unchanged from
0.5.0; this milestone intentionally does not change the release install path.
Continue the outstanding **M0 performance/reboot/rollback evidence** in
parallel, without labeling that gate complete on the strength of M1 schema tests.
