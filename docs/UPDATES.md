# Release updates

Open **Settings → Updates** to configure release checks or choose **Check now**.
Automatic checking defaults to **On**, every **24 hours**. Available intervals
are 15 minutes, 30 minutes, 1 hour, 3 hours, 6 hours, 12 hours, and 24 hours.

When enabled, RoundOS checks at startup and then at the selected interval.
Turning automatic checks off leaves Check now available. All release-check
attempts, including failed checks and startup checks, share a persisted
15-minute cooldown. Restarting RoundOS does not bypass it. GitHub rate limits
can require a longer wait; the screen shows the next eligible check. Changing
the interval does not clear the cooldown or discard a pending startup check.

## Available releases and notes

RoundOS checks the public `drazafsky/roundos` GitHub repository for a newer stable
release. Drafts and development prereleases are excluded. A stable version can
replace a development build with the same numeric version.

The available-release prompt offers:

- **Install:** install this selected release on a supported managed device.
  RoundOS will restart; the device is not rebooted.
- **Cancel:** close the prompt. A later successful check may offer it again.
- **Release notes:** open wrapped text that scrolls with touch or the encoder.
  Close or Back returns to the same choices. Reopening preserves the scroll
  position and the release being reviewed.
- **Ignore release:** save the choice and suppress automatic prompts for this
  version across restarts. A newer release can still prompt. Check now can
  offer an ignored release again. A save failure remains visible for retry.

Automatic prompts wait for editors, confirmations, overlays, dialogs, and active
input. A check that finishes while notes are open does not replace those notes.
Closing the current prompt can then offer a newer queued release.

## Installation

Checking alone never downloads an installation package or installs software.
Choosing Install sends the selected release identity and asset digests to an
independent helper. Preparation downloads and validates the package while the
current shell remains usable. Installation controls reject duplicate requests.
Once installation starts, **Close** dismisses the prompt while the update
continues.

The helper stops RoundOS only after validation. It preserves the current,
previous, and recovery-controller release links and the managed service units,
installs the candidate, and waits for its readiness notification. Readiness
requires a presented frame and initial Clock and Timer views. If startup fails,
the helper restores the old release and units and starts the old shell.
Settings, application state, Wi-Fi profiles, and the service activation policy
are preserved.

Settings shows preparation, installation, and recovery status. After restart,
the persisted result is loaded and a notification reports installation,
preparation failure, rollback, or the need for SSH recovery. A rollback restores
the prior version; it does not silently retry the failed installation.

Install requires a managed 32-bit ARM Linux device running the readiness-capable
systemd service and independent update controller. Development, preview, and
unmanaged sessions retain checking and notes while installation is disabled.
Custom unit files, systemd drop-ins, or a missing/incompatible controller require
a manual managed update. A recovery-required transaction must be repaired over
SSH before another installation is allowed.

An older installation needs an explicit bootstrap using a package containing
the new helper units and update protocol. Follow the installation and activation
steps in [the managed startup runbook](SYSTEMD-ROLLBACK.md). The standalone
installer requires RoundOS to be inactive or failed; the managed helper handles
that stop itself. Installing a package does not enable RoundOS or change the
desktop activation policy.

Manual installation and mutating `roundosctl` operations share the update
transaction lock. Manual installation, activation, and rollback also reject an
unfinished transaction after its helper has stopped. Let a running update finish,
or use SSH recovery before another installation or activation. The desktop escape
command, `roundosctl disable`, remains available once the helper releases its lock.

## Stable package requirements

A compatible release contains these assets:

- `roundos-VERSION-linux-armv7.tar.gz`
- `release.json`
- `SHA256SUMS`

Each asset must have a GitHub SHA-256 digest. Downloads are pinned to the digests
captured during discovery, so replacing files under the same tag does not change
the selected package. Releases with missing digests or incompatible packages
cannot be installed.

Validation checks asset and archive manifests, matching stable version and full
commit identity, required installer files, executable permissions, ARM executable
structure, and managed readiness/update protocols. Extraction rejects traversal,
duplicates, links, and special files. Downloads, archive bytes and metadata,
filesystem objects, and runtime-probe output are bounded. Failed preparation
removes its private staging directory and leaves the running release unchanged.

Stable publication and Pi acceptance are separate from checking and installing;
see [CI and releases](CI-RELEASES.md).

## Troubleshooting and SSH recovery

If checking fails, confirm network access and wait for the displayed cooldown or
GitHub retry deadline. If preferences cannot be saved, inspect free space and
write access to `/var/lib/roundos/shell-data`. The unresolved storage error stays
visible until a successful write.

For installation problems, inspect the independent helper and recovery logs:

```sh
sudo systemctl status roundos.service roundos-update.service roundos-update-recover.service --no-pager
sudo journalctl -u roundos-update.service -u roundos-update-recover.service -u roundos.service -n 150 --no-pager
sudo roundosctl status
sudo roundosctl verify
```

The private transaction is
`/var/lib/roundos/shell-data/update-install.json`. Download staging is under
`/var/lib/roundos/update-cache`. The controller is
`/opt/roundos/update-controller`, independent of `/opt/roundos/current`.
Failed installed candidates are retained under `.failed-*` release directories.

For a failed or stopped helper that needs recovery, restore the existing
transaction over SSH, then start the restored service explicitly:

```sh
sudo systemctl stop roundos-update.service
sudo /opt/roundos/update-controller/bin/roundd -recover-update
sudo systemctl reset-failed roundos-update-recover.service roundos.service
sudo systemctl restart roundos-update-recover.service
sudo systemctl start roundos.service
sudo roundosctl verify
```

The recovery command restores links and units without starting/stopping its
dependent shell. This avoids a boot ordering deadlock; the explicit service
start above is for manual recovery. At boot, systemd starts the restored shell.
If only the outcome journal is unwritable after safe restoration, boot can
proceed and the service logs report the persistence failure. Fix storage access and
rerun recovery to record the terminal outcome; the old progress record may
remain visible until then.

If the controller, backups, or installed files cannot be verified, use the
[managed rollback and desktop recovery runbook](SYSTEMD-ROLLBACK.md). Retain the
transaction and logs for diagnosis. Do not delete shell/app state or Wi-Fi
profiles to clear an update failure.

Physical acceptance is tracked in [the update acceptance record](UPDATE-ACCEPTANCE.md).
