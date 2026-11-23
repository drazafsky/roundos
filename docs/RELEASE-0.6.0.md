# RoundOS 0.6.0 candidate: declarative UI

Release status: development candidate; Pi acceptance and stable promotion pending.

Clock and Timer now define their screens in compiled RUI YAML and retain Go
state/actions, existing data formats and background behavior. The public SDK
supports typed bindings, all existing widgets, shell-owned templates and optional
revisioned property updates. Unmodified protocol-v7 apps retain full-view fallback.

The roundui compiler validates, compiles, inspects and previews layouts. Explicit
manual development sessions support trusted SSH hot reload with last-good state,
focus preservation and editor deferral. Managed production runs use embedded
bundles; they enable no development watcher or YAML runtime parser.

See rui-m6/START-HERE.md for migration and external SDK builds, and
rui-m7/START-HERE.md for exact-artifact Pi performance, physical controls,
installation and rollback acceptance. Keep managed 0.5.0 and its original control
files available until those checks pass. No irreversible state migration is needed.
