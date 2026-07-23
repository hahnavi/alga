#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

mkdir -p "$TMP_DIR/.hermes/profiles/inve9" "$TMP_DIR/bin" "$TMP_DIR/venv/bin"

cat > "$TMP_DIR/venv/bin/python3" <<'PY'
#!/usr/bin/env bash
exit 0
PY
chmod +x "$TMP_DIR/venv/bin/python3"

cat > "$TMP_DIR/venv/bin/pip" <<'PIP'
#!/usr/bin/env bash
exit 0
PIP
chmod +x "$TMP_DIR/venv/bin/pip"

cat > "$TMP_DIR/bin/hermes" <<'HERMES'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$HERMES_LOG"
HERMES
chmod +x "$TMP_DIR/bin/hermes"

HERMES_HOME="$TMP_DIR/.hermes" \
HERMES_VENV="$TMP_DIR/venv" \
HERMES_LOG="$TMP_DIR/hermes.log" \
PATH="$TMP_DIR/bin:$PATH" \
    bash "$ROOT_DIR/install.sh" --profile inve9 > "$TMP_DIR/install.out"

grep -qx -- '--profile inve9 plugins enable alga-platform' "$TMP_DIR/hermes.log"
grep -qx -- '--profile inve9 tools enable alga' "$TMP_DIR/hermes.log"

if grep -q 'plugin: hermes --profile inve9 plugins enable alga-platform' "$TMP_DIR/install.out"; then
    echo "plugin enable command should not remain manual" >&2
    exit 1
fi

if grep -q 'toolset: hermes --profile inve9 tools enable alga' "$TMP_DIR/install.out"; then
    echo "toolset enable command should not remain manual" >&2
    exit 1
fi
