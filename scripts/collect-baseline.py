#!/usr/bin/env python3
"""Read-only RoundOS performance sampler: JSONL, never captures passwords/SSIDs.

Run over SSH while the managed service is active. Process CPU percentages are
interval deltas over a single logical CPU, so multicore usage may exceed 100%.
"""
import argparse
import datetime
import json
import os
import pathlib
import time

NAMES = {"roundd", "round-clock", "round-timer", "round-demo"}


def read_tasks(proc="/proc"):
    samples = {}
    clock = os.sysconf("SC_CLK_TCK")
    page_bytes = os.sysconf("SC_PAGE_SIZE")
    for entry in pathlib.Path(proc).iterdir():
        if not entry.name.isdecimal():
            continue
        try:
            content = (entry / "stat").read_text()
            split_at = content.rfind(")")
            fields = content[split_at + 1 :].split()
            name = content[content.find("(") + 1 : split_at]
            if name not in NAMES:
                continue
            # procfs fields after pid and comm: state is 3, utime 14, stime 15,
            # starttime 22, rss 24. Index relative to state starts at zero.
            cpu_ticks = int(fields[11]) + int(fields[12])
            start_ticks = int(fields[19])
            rss_kib = int(fields[21]) * page_bytes // 1024
            samples[f"{name}:{entry.name}:{start_ticks}"] = {
                "name": name, "pid": int(entry.name), "cpu_seconds": cpu_ticks / clock,
                "rss_kib": rss_kib,
            }
        except (OSError, ValueError, IndexError):
            # Process may exit between /proc traversal and file reading.
            continue
    return samples


def memory_available_kib():
    try:
        for line in pathlib.Path("/proc/meminfo").read_text().splitlines():
            if line.startswith("MemAvailable:"):
                return int(line.split()[1])
    except (OSError, IndexError, ValueError):
        pass
    return None


def temperature_c():
    try:
        return round(int(pathlib.Path("/sys/class/thermal/thermal_zone0/temp").read_text().strip()) / 1000, 1)
    except (OSError, ValueError):
        return None


def sample(prev, current, elapsed):
    observations = []
    for key, data in sorted(current.items()):
        prior = prev.get(key)
        cpu = None
        if prior is not None and elapsed > 0:
            cpu = round(max(0, data["cpu_seconds"] - prior["cpu_seconds"]) * 100 / elapsed, 2)
        observations.append({"name": data["name"], "pid": data["pid"],
                             "rss_kib": data["rss_kib"], "cpu_percent": cpu})
    return observations


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--seconds", type=int, default=60, help="seconds of sampling (1-3600)")
    parser.add_argument("--interval", type=float, default=1, help="sampling interval seconds (0.2-30)")
    parser.add_argument("--output", required=True, help="new JSONL output path")
    args = parser.parse_args()
    if not 1 <= args.seconds <= 3600 or not 0.2 <= args.interval <= 30:
        parser.error("seconds must be 1-3600 and interval must be 0.2-30")
    dest = pathlib.Path(args.output)
    fd = os.open(str(dest), os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    previous = read_tasks()
    start = time.monotonic()
    last = start
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as f:
            while time.monotonic() - start < args.seconds:
                time.sleep(min(args.interval, max(0, args.seconds - (time.monotonic()-start))))
                now = time.monotonic()
                current = read_tasks()
                record = {
                    "utc": datetime.datetime.now(datetime.timezone.utc).isoformat(),
                    "elapsed_seconds": round(now-start, 3),
                    "temperature_c": temperature_c(),
                    "memory_available_kib": memory_available_kib(),
                    "load_1m": round(os.getloadavg()[0], 3),
                    "processes": sample(previous,current,now-last),
                }
                f.write(json.dumps(record, separators=(",", ":")) + "\n")
                f.flush()
                previous,last = current,now
    except BaseException:
        dest.unlink(missing_ok=True)
        raise
    print(f"Recorded non-secret performance data to {dest}")
    print("RoundOS render/input-to-present p95 values are logged separately by the service.")


if __name__ == "__main__":
    main()
