# RoundOS 0.5-rc4-wifi — Privilege-separated application milestone

**Baseline:** Exact `roundos-0.5-rc3-wifi.zip` source. This RC4 candidate is a separate directory and does not modify previously tested installations, kernel modules, display overlays, boot splash, NetworkManager profiles, or systemd service definitions.

## Goal

Reduce the impact of a compromised or malfunctioning optional application. The DRM shell still needs elevated access to `/dev/dri`, input devices and NetworkManager. The built-in Clock/Timer and optional trusted applications **must not inherit root UID/GID or root shell environment**. The shell must refuse a privileged launch unless the app account has been provisioned.

## Implementation

- `roundd` requires `-app-user roundos-app` on root launches. Startup fails instead of silently launching applications as root. The default app data directory is `/var/lib/roundos/app-data`.
- `scripts/setup-app-user.sh` explicitly creates a dedicated system user with `nologin`, no home directory, and a private 0700 application state directory. It attempts a **non-overwriting** migration of the historical root-launched Timer state from `/root/.local/state/roundos/timer.json`, preserving the original.
- Apps are launched with `syscall.Credential{Uid,Gid,Groups:[]}` (supplementary groups cleared); no access to the shell's inherited environment. App environment contains only the private IPC socket/token, app ID, state directory, a minimal PATH, and locale/user settings.
- IPC directory is root-owned and group-traversable (0710); IPC socket is root-owned, group-accessible (0660). Linux `SO_PEERCRED` checks the connecting UID against the app account **in addition to** the existing per-app random authentication tokens.
- Existing app state is available only in `/var/lib/roundos/app-data` (owned by `roundos-app:roundos-app`, 0700). Credentials, NetworkManager profiles and DRM/input device access remain with the root shell, not applications.
- `scripts/install-app-binaries.sh` copies freshly built Clock/Timer binaries to root-managed `/usr/local/libexec/roundos` with mode 0755. The test launcher invokes these installed binaries, avoiding failures when the original source is inside a 0700 home directory.
- The `test-drm.sh` launcher preserves the previous LightDM stop/restore behavior, verifies the runtime is provisioned before stopping the desktop, and passes explicit app user/path options.
- All existing `AppSpec` trusted-manifest discovery logic, user-facing Wi-Fi settings, touch/rotary gestures, themes, and shell-owned dialogs remain present.

## First RC4 setup (SSH)

```sh
cd ~/roundos-0.5-rc4-wifi
./scripts/build.sh
./scripts/setup-app-user.sh
./scripts/test-drm.sh -gesture-config config/gestures.example.json
```

The setup command is **explicit** and only needs to run once. The test script reinstalls the matching built-in binaries after each build. With `sudo` correctly configured it does not create or modify any systemd unit.

After RoundOS starts, from a second SSH terminal run:

```sh
cd ~/roundos-0.5-rc4-wifi
./scripts/verify-app-privileges.sh
./scripts/collect-perf.sh
```

The first script reports per-process UIDs/GIDs without printing IPC tokens or Wi-Fi credentials.

## Tests to perform on Raspberry Pi Zero 2 W

- [ ] First build and `setup-app-user.sh` succeed; `getent passwd roundos-app` resolves a non-root UID, and `/var/lib/roundos/app-data` is mode 0700, owned by that user.
- [ ] `test-drm.sh` starts Clock and Timer normally. The launcher is still root and app PIDs run as `roundos-app`. `verify-app-privileges.sh` passes.
- [ ] The Timer's previous remaining/running state is retained after migration, if it existed. Start it, switch to Clock, restart Timer with `x` and check recovery; inspect state mode 0600 and app user ownership.
- [ ] Clock's label editor works and saves in memory; the new account does not change the QWERTY/rotary interaction.
- [ ] Wi-Fi scanning, network ordering, password entry, Remember and reconnection still work (Wi-Fi operations are shell-side, not app processes).
- [ ] Previously installed Demo or other **reviewed** manifests are discovered; the app opens and its IPC works as `roundos-app`.
- [ ] Staggered two-finger horizontal and Home gestures; Back; notifications; launcher; app switcher; dialogs/pickers; kinetic scrolling; and themes show no regression.
- [ ] SSH `s` metrics plus `./scripts/collect-perf.sh` are recorded under active and idle load. Performance thresholds remain **not certified** until actual Pi measurements are reviewed.
- [ ] `q` exits, and LightDM is restored. RC3 still starts independently if rollback is necessary.

## Security boundary and known limits

This is **UID-level privilege separation**, **not a security sandbox**. All apps currently share the same `roundos-app` UID and state directory, so an untrusted app could read/modify another app's state or inspect processes owned by that account, including potentially accessing another app's IPC authentication token. The system does **not** yet provide per-app UID namespaces, seccomp, cgroups, filesystem sandboxing, capability restrictions, or setuid-execution protection. Do not install arbitrary downloaded/unreviewed applications. Only install trusted admin-managed binaries, as in RC3.

The DRM shell is still root and NetworkManager credentials are managed through protected system profiles, not a RoundOS password database. App isolation does not protect against defects in privileged shell/NetworkManager code.

This is a release candidate for physical testing, **not final RoundOS 0.5**, until Pi hardware validation and actual performance results are available.

## Rollback

Keep the RC3 directory untouched. Exit RC4 using `q` and restart the RC3 test launcher. The new system account and state directory can remain installed harmlessly; RC3 does not need them. Do not remove `/var/lib/roundos/app-data` if it contains Timer state. The previous Timer state file remains untouched when it is migrated.
