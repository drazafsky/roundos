# M0.1 Pi verifier compatibility fix

This patch changes only `scripts/m0/verify_source.py`, `scripts/m0/verify_host.py`, and their M0 test module. It does **not** alter any of the 149 verified RoundOS 0.5.0 baseline files, the installed service, binaries, or driver.

## Why these checks fail on the Pi

- The official `./scripts/build.sh` writes `roundd`, `round-clock`, and `round-timer` ELF binaries into the project directory. The previous provenance checker incorrectly treated those legitimate build outputs as unexpected source changes. The new checker permits **only** these three build outputs when they are regular ELF files, while continuing to reject modifications of pinned source files and unexpected source additions.
- The M0 Python test calls the same provenance checker, so its failure can be secondary to the above. The other tests must still be checked individually.
- Go's `-race` flag is unsupported on `linux/arm` (32-bit Linux on Pi Zero 2 W). The new verifier marks this check `SKIP_UNSUPPORTED` and the overall report `PARTIAL` if no other checks fail. This is not a substitute for race tests on an amd64/arm64 build host.
- Failures now print the corresponding evidence log filename.

## Apply in-place (from the existing M0 checkout)

Extract `roundos-rui-m0-pi-verifier-fix.zip` **inside** `~/roundos-0.5.0-rui-m0`:

```sh
cd ~/roundos-0.5.0-rui-m0
unzip -o ~/roundos-rui-m0-pi-verifier-fix.zip
python3 scripts/m0/verify_source.py
python3 -m unittest discover -s scripts/m0/tests -p 'test_*.py'
python3 scripts/m0/verify_host.py --output "$HOME/roundos-m0-host-verify-$(date +%Y%m%d-%H%M%S)"
```

A Pi run with otherwise successful tests should report `provenance PASS`, `m0_python_tests PASS`, `go_race SKIP_UNSUPPORTED`, and `RESULT: PARTIAL`. This means all **platform-supported** host checks passed, but race testing and the hardware acceptance checklist remain separate. The `report.py` M0 gate still requires supported-host race evidence and Pi acceptance measurements; it is not marked automatically complete.

If provenance or M0 Python tests continue to fail, inspect the new evidence directory's `provenance.log` and `m0_python_tests.log` and report the exact error. The checker intentionally refuses any unexpected additions other than its very narrow whitelist.
