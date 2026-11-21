# M4: trusted desktop-to-Pi layout reload

Production continues to use embedded bundles. Installing this release enables
no watcher, YAML parser, development session, or new network listener.

`roundui dev` polls one regular non-symlink source file every 100 ms and
debounces changes for 150 ms. It compiles on the desktop and sends only bounded
compiled bundles over persistent SSH to `roundui dev-agent`. The agent reads
the secret from a private session file and forwards the candidate to the app's
Unix socket. Host-key verification and key authentication remain enabled.
Secrets and bundle data never appear in shell command arguments. Only the
latest candidate is retained across failures.

## Separate manual development session

Build the reviewed source and demo in a separate Pi workspace with native Go,
pkg-config and libdrm development headers:

```sh
cd ~/roundos-dev/source
ROUNDOS_OUTPUT_DIR="$HOME/roundos-dev/bin" sh scripts/build.sh
go build -o ~/roundos-dev/bin/round-rui-demo ./examples/rui-demo
mkdir -p ~/roundos-dev/apps ~/roundos-dev/state/shell
install -m 0755 ~/roundos-dev/bin/round-rui-demo ~/roundos-dev/apps/round-rui-demo
printf '%s\n' '{"id":"org.example.rui.demo","name":"RUI Demo","icon":"RUI","exec":"round-rui-demo"}' > ~/roundos-dev/apps/demo.json
chmod 0644 ~/roundos-dev/apps/demo.json
install -d -m 0700 ~/roundos-dev/control
```

Arrange a display maintenance window, stop the installed service over SSH, and
launch the separate shell as the non-root developer with existing video/input
device permissions:

```sh
sudo /usr/bin/systemctl stop roundos.service
~/roundos-dev/bin/roundd -backend drm -card /dev/dri/card0 -connector DPI-1 \
  -clock-exec "$HOME/roundos-dev/bin/round-clock" \
  -timer-exec "$HOME/roundos-dev/bin/round-timer" \
  -apps-dir "$HOME/roundos-dev/apps" -state-dir "$HOME/roundos-dev/state/shell" \
  -rui-dev-app org.example.rui.demo -rui-dev-dir "$HOME/roundos-dev/control"
```

This is the existing same-user manual development flow, not the production
dedicated-app-user isolation. A root-launched manual shell can retain
`-app-user` isolation using a pre-provisioned directory owned by that app user.
Managed systemd sessions reject dev flags. Other apps never inherit the opt-in.
Production manifests, release symlinks and persistent app data stay intact.

The demo prints its mode-0600 `session.json` path inside a fresh mode-0700
directory. Copy the path, never its contents, into the desktop command. Select
the RUI demo on the Pi and run on the desktop:

```sh
roundui dev examples/rui-demo/layout.rui.yaml \
  -ssh drazafsky@192.168.1.6 \
  -remote-tool /home/drazafsky/roundos-dev/bin/roundui \
  -session /home/drazafsky/roundos-dev/control/rui-EXAMPLE/session.json
```

If the app uses a separate account, `-sudo-user roundos-app` runs the remote
agent as that account; it requires an administrator-reviewed remote helper and
an explicit sudo rule. The agent does not run as root. For local delivery omit
`-ssh` and `-remote-tool`. Ctrl+C reaps SSH. Disconnected delivery retries with
bounded state. App restarts create a fresh session path; restart the desktop
command using that new path. After stopping the manual shell, restore the
installed service with `sudo systemctl start roundos.service`.

## SDK integration

An app explicitly starts `rui.ListenDev(ctx, privateDirectory)` and selects on
`session.Updates()` alongside its SDK events. It calls `update.Apply(runtime)`
on the same event loop as Dispatch and Flush. The transport neither consumes
SDK events nor invokes handlers; application state and registrations survive.
The listener requires the matching `ROUNDOS_RUI_DEV_DIR` opt-in supplied by a
manual supervisor launch; normal inherited environments are filtered out.

`Runtime.Replace(compiled)` stages semantic, identity, registration, dynamic
value and frame-budget checks before replacing its tree. Bad candidates leave
the runtime and visible UI intact. Getter panics during staging become
sanitized errors. Existing registrations remain available for future layouts.

`Client.SecureEditorActive()` reports only editing status. Replacement returns
`rui.ErrReloadDeferred` while a secure editor is open, and the desktop retries
its latest candidate. Stable identities preserve navigation and compatible
drafts. Removed focus chooses the nearest eligible old neighbor (preceding
wins ties), then a new eligible control, or Back. Incompatible picker/slider
transactions and structural kinetic scrolling are canceled without commits.

## Verification

`sh scripts/m4/verify.sh` runs Python regressions, Go tests/vet and RUI/ARMv7
checks. Main CI also runs race and DRM checks. Tests cover invalid reloads,
secure editing, private control sessions, strict framing, SSH quoting/reuse/
cancellation, debounce, last-good state, navigation and compatible drafts.

Actual Pi display latency, touch/encoder operation, secure-editor observations
and the 500 ms reload gate remain physical acceptance work. A control-channel
acknowledgement alone does not prove that a frame was displayed.
