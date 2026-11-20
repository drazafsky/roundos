# Security boundary — RoundOS 0.5.0

## Host privileges

The DRM shell intentionally runs as root to access DRM/KMS, evdev and NetworkManager. Clock, Timer and user-reviewed optional applications run as `roundos-app` via Unix `setuid`/`setgid` process credentials. The parent strips inherited privileged environment variables; IPC uses authenticated Unix peer UIDs plus per-process tokens, with restricted socket permissions. The systemd unit adds read-only system areas, restricted home access, a private temporary directory, and no new privileges.

This is **not** a security sandbox between apps. All apps share the `roundos-app` UID and the private app data directory and may interfere with one another. Do not install untrusted applications or assume confidentiality between apps. Trusted manifests and executables must be root-managed under `/etc/roundos/apps`, with no writable parents and no symlink escapes.

## Wi-Fi credentials

Wi-Fi scanning and activation are delegated to NetworkManager. Previously connected networks come from saved NetworkManager profiles, displayed ahead of new available SSIDs. Passwords are transmitted to `nmcli` through temporary restricted input rather than command arguments; RoundOS does not put Wi-Fi secrets into appearance preferences or app IPC. NetworkManager is the authority for persistent credentials. A password editor is masked; high-frequency touch trace is suppressed during secure text entry. No Wi-Fi credentials are copied into systemd units or performance logs.

## Data and rollback

`/var/lib/roundos/app-data` remains mode 0700, owned by `roundos-app`. `/var/lib/roundos/shell-data` remains owned by root, mode 0700. Managed releases are immutable root-owned binaries under `/opt/roundos/releases`; checksum verification detects accidental/tampered binary changes after installation, but does not sign or authenticate the original source archive. Keep trusted release source archives and ensure installers are reviewed before running with `sudo`. Upgrade/rollback never removes NetworkManager profiles, Timer state or unrelated user directories.

## Remaining hardening for 0.6

Per-application UIDs, permissions, app sandboxes (e.g., user namespaces/systemd sandbox per app), secrets isolation between apps, signed app manifests, and a stable external Go module distribution model are future work rather than claims of this 0.5 release.
