#!/bin/sh
# Explicitly opt-in to a trusted example application. Never run this as part
# of the normal build or RoundOS startup. Only install code you have reviewed.
set -eu
cd "$(dirname "$0")/.."
GOFLAGS='' go build -o round-demo ./cmd/round-demo
sudo install -d -o root -g root -m 0755 /etc/roundos/apps
sudo install -o root -g root -m 0755 ./round-demo /etc/roundos/apps/round-demo
printf '%s\n' '{"id":"org.roundos.demo","name":"Demo","icon":"DME","exec":"round-demo"}' | sudo tee /etc/roundos/apps/org.roundos.demo.json >/dev/null
sudo chown root:root /etc/roundos/apps/org.roundos.demo.json
sudo chmod 0644 /etc/roundos/apps/org.roundos.demo.json
printf 'Installed trusted demo. Restart RoundOS to discover it.\n'
