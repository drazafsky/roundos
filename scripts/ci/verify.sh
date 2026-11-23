#!/bin/sh
# All automated host checks. Physical Pi acceptance is a separate gate.
set -eu
cd "$(dirname "$0")/../.."
python3 -m unittest discover -s scripts/m0/tests -p 'test_*.py'
python3 -m unittest discover -s scripts/m7/tests -p 'test_*.py'
python3 -m unittest discover -s scripts/tests -p 'test_*.py'
python3 -m unittest discover -s scripts/ci/tests -p 'test_*.py'
python3 - <<'PY'
from pathlib import Path
import subprocess
for path in sorted(Path('scripts').rglob('*.sh')):
    subprocess.run(['sh', '-n', str(path)], check=True)
PY
go test ./... -count=1
go test -race ./... -count=1
go test -tags drm ./... -count=1
go vet ./...
sh scripts/ci/verify-rui.sh
