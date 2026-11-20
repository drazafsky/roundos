# RoundOS 0.5.0 on-device performance baseline

The Pi Zero 2 W with 480×480 HyperPixel DPI panel is the reference platform. Host Go tests and PNG previews prove neither actual touchscreen latency nor thermal/power performance.

## Passive automatic shell metrics

The systemd launcher passes `-metrics-every 60s`. RoundOS writes to the journal:

- application and shell resident memory (MiB)
- frame p95 and input-to-present p95 for frames that the presenter has completed
- number of visual frames coalesced by the asynchronous presenter

These are process-level/rendering measurements; event-to-present timing excludes electrical touch-controller IRQ-to-pixel-light delay. Do not label them end-to-end physical latency.

Run `sudo journalctl -u roundos.service --since '10 minutes ago' --no-pager | grep -E 'RoundOS performance|RSS|RoundOS stats'` to review summaries. Do not post full journal logs without reviewing for identifying device/SSID information.

## Read-only host samples

In a second SSH session while the service is running:

```sh
python3 scripts/collect-baseline.py --seconds 60 --output ~/roundos-idle.jsonl
```

Then repeat under a fast Clock widget gallery flick, Timer running in the background, Wi-Fi Settings scan, and with Reduce Motion enabled. The `.jsonl` output stores only timestamp, elapsed time, CPU percentage, RSS, temperature, available memory and load; no SSID, password, input coordinate, process arguments, or environment data. File is created exclusively (fails if it exists), mode 0600.

Collect ideally three independent 60-second samples per condition to observe variability. Report whether app, shell and thermal measurements remain reasonable for long-term use. There is **no** performance number certified for 0.5.0 until the user's physical Pi measurements are available.

## Power and idle

Input loop should not require continuous redraw of static screens: clock Home refresh is minute-oriented; notifications, touch animations and gestures schedule additional work when necessary. Backlight power management requires an available kernel backlight controller and an explicit `-backlight ... -idle-off ...` configuration, not assumed by default. If the kernel/device cannot expose brightness, display-off functionality must not be asserted as tested.
