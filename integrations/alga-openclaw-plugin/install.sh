#!/usr/bin/env bash
# Install / sync / uninstall the Alga plugin for OpenClaw.
#
# Thin wrapper around install.js. Detects the system Node, falls back to common
# install paths, and forwards all arguments.
#
# Usage:
#   bash install.sh                      # install or sync to default profile
#   bash install.sh --profile <name>     # install or sync to custom profile
#   bash install.sh --status             # check installation status
#   bash install.sh --uninstall          # remove plugin
#
# All real work is in install.js (idempotent).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# Pick a Node.js 22+ binary. Honor $NODE first, then PATH lookups.
find_node() {
    if [[ -n "${NODE:-}" ]] && [[ -x "$NODE" ]]; then
        echo "$NODE"; return 0
    fi
    local candidates=(node node22 nodejs22 nodejs)
    for c in "${candidates[@]}"; do
        if command -v "$c" >/dev/null 2>&1; then
            echo "$c"; return 0
        fi
    done
    return 1
}

NODE_BIN="$(find_node || true)"
if [[ -z "$NODE_BIN" ]]; then
    echo -e "\033[0;31m[alga]\033[0m Node.js 22+ is required but not found in PATH." >&2
    exit 1
fi

exec "$NODE_BIN" "$SCRIPT_DIR/install.js" "$@"
