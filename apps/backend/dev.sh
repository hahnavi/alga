#!/usr/bin/env bash
set -euo pipefail

AIR_BIN="$(go env GOPATH)/bin/air"

if ! [ -x "$AIR_BIN" ]; then
    echo "Installing air..."
    go install github.com/air-verse/air@latest
fi

exec "$AIR_BIN" -c .air.toml
