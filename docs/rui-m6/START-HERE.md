# M6: built-in applications and public SDK

Clock and Timer now load `layouts/clock.rui.bin` and `layouts/timer.rui.bin`.
Their complete presentation lives in the adjacent YAML, including Clock's
options, gallery, pickers and help, and Timer's duration controls and reset
confirmation. Stable screen/widget/action IDs and legacy scalar presentation
are preserved. Timer's scalar `adjust` compatibility route remains an adapter;
its state, scheduling, notifications, persistence and crash diagnostic remain Go.

Run `go generate ./layouts` after editing either YAML file. CI recompiles and
compares both checked-in artifacts, validates previews, and builds ARMv7.
Development reload uses the same explicitly enabled private sessions described
in [M4](../rui-m4/START-HERE.md); managed production uses embedded layouts.
Both apps apply reloads, dispatch input and flush on their existing serial loop.

## Startup and bindings

`Runtime.PublishInitial()` publishes one validated readiness snapshot while an
app is hidden. It does not change foreground selection. Repeated calls after
successful publication do nothing; failures may be retried. Ordinary `Flush()`
continues to defer hidden updates. Built-in apps call this after loading state
and validating every registration, allowing managed startup to verify readiness.

Integer `min` and `max` properties accept either literals or `{ bind: name }`.
This additive v1 extension preserves Clock's startup-date ±365-day picker range
without generating UI trees in Go. It uses existing int registrations, compiler
dependency indices and runtime/preview validation. `timePicker` retains its fixed
0..1439 range. Bound ranges and their value update as a consistency unit; an
invalid combination retains the previous valid unit while unrelated bindings
can update. The bindings stay dirty for retry. A failed initial evaluation
publishes nothing. Range changes use a new full layout revision and cancel
incompatible drafts. Legacy integer actions are checked against published bounds.

Root title, hero, subtitle and primary-button metadata also provide the legacy
scalar fields. Preview and runtime use the same lowering. Applications provide
domain getters and registered handlers; they do not construct widget trees.

## Independent application build

The repository currently uses Go module name `roundos`. A standalone application
can use a local SDK checkout through a Go module replacement:

```sh
mkdir my-roundos-app
cd my-roundos-app
go mod init example.org/my-roundos-app
go mod edit -require=roundos@v0.0.0
go mod edit -replace=roundos=/absolute/path/to/roundos
cp /absolute/path/to/roundos/examples/rui-demo/main.go .
cp /absolute/path/to/roundos/examples/rui-demo/layout.rui.yaml .
/absolute/path/to/roundui compile -o layout.rui.bin layout.rui.yaml
go build .
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build .
```

Keep the application's RUI application ID and trusted manifest ID identical.
The example imports only public `sdk/app` and `sdk/rui` packages, embeds its
compiled resource, registers getters/handlers, and owns one `Client.Events()`
consumer. Call `Invalidate` for state changes and `Flush` on that loop. Getter
errors retain last-good state; handlers return errors to reject optimistic input.
Forward editor results through `Dispatch` before applying them to Go state, then
invalidate the affected keys. Do not use a second goroutine to consume events.

`sdk/rui` tests build this example from a separate temporary Go module to guard
the SDK boundary. Process integration tests compare migrated controls and tested
actions directly with independently compiled original 0.5.0 sources, and retain
the existing editing, background deadline, readiness and crash-recovery tests.

Run `sh scripts/m6/verify.sh`. Physical display/touch/encoder, performance,
installation and rollback acceptance remain M7 gates.
