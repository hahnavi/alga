#!/usr/bin/env bash
# Install / sync / uninstall the Alga platform plugin for Hermes Agent.
#
# Usage:
#   bash install.sh                      # install or sync to default profile
#   bash install.sh --profile <profile>  # install or sync to custom profile
#   bash install.sh --status             # check installation status
#   bash install.sh --uninstall          # remove plugin

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
DIM='\033[2m'
BOLD='\033[1m'
NC='\033[0m'

info()  { echo -e "${GREEN}[alga]${NC} $*"; }
warn()  { echo -e "${YELLOW}[alga]${NC} $*"; }
err()   { echo -e "${RED}[alga]${NC} $*" >&2; }
die()   { err "$@"; exit 1; }

usage() {
    echo -e "${BOLD}Usage:${NC}"
    echo -e "  bash install.sh [options]"
    echo ""
    echo -e "${BOLD}Options:${NC}"
    echo -e "  --profile <name>  Install to custom profile (default: default profile)"
    echo -e "  --status          Check installation status"
    echo -e "  --uninstall       Remove plugin"
    echo -e "  -h, --help        Show this help message"
}

PROFILE="default"
ACTION="install"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --profile)
            if [[ -z "${2:-}" ]]; then
                die "Error: --profile requires a non-empty argument"
            fi
            PROFILE="$2"
            shift 2
            ;;
        --status)
            ACTION="status"
            shift
            ;;
        --uninstall)
            ACTION="uninstall"
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            err "Error: Unknown option '$1'"
            usage
            exit 1
            ;;
    esac
done

# Determine default/base HERMES_HOME.
# If the inherited HERMES_HOME environment variable is already a profile directory,
# find its base directory (e.g. /home/solo/.hermes/profiles/comma5 -> /home/solo/.hermes).
BASE_HERMES_HOME="${HERMES_HOME:-$HOME/.hermes}"
if [[ "$BASE_HERMES_HOME" =~ /profiles/[^/]+$ ]]; then
    BASE_HERMES_HOME="$(dirname "$(dirname "$BASE_HERMES_HOME")")"
fi

if [ "$PROFILE" = "default" ]; then
    HERMES_HOME="$BASE_HERMES_HOME"
else
    HERMES_HOME="$BASE_HERMES_HOME/profiles/$PROFILE"
fi

PLUGIN_NAME="alga-platform"
PLUGIN_DIR="$HERMES_HOME/plugins/$PLUGIN_NAME"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SOURCE_DIR="$SCRIPT_DIR/plugin"

# Venv resides in the base/default HERMES_HOME (e.g., ~/.hermes/hermes-agent/venv)
HERMES_VENV="${HERMES_VENV:-$BASE_HERMES_HOME/hermes-agent/venv}"
HERMES_PYTHON="$HERMES_VENV/bin/python3"
HERMES_PIP="$HERMES_VENV/bin/pip"

PLUGIN_FILES=(plugin.yaml __init__.py register.py)


check_file_sync() {
    local src="$1" dst="$2"
    if [ ! -f "$dst" ]; then
        return 1
    fi
    cmp -s "$src" "$dst"
}

check_httpx() {
    if [ -x "$HERMES_PYTHON" ]; then
        "$HERMES_PYTHON" -c "import httpx" 2>/dev/null
    elif command -v python3 &>/dev/null; then
        python3 -c "import httpx" 2>/dev/null
    else
        return 1
    fi
}

check_env_set() {
    [ -f "$HERMES_HOME/.env" ] && grep -q "^ALGA_SERVER_URL=\S" "$HERMES_HOME/.env" 2>/dev/null
}

run_hermes() {
    command -v hermes >/dev/null 2>&1 || die "Hermes CLI not found in PATH. Install Hermes Agent first."

    if [ "$PROFILE" = "default" ]; then
        hermes "$@"
    else
        hermes --profile "$PROFILE" "$@"
    fi
}

# ── Status ──────────────────────────────────────────────────────────────

show_status() {
    local all_ok=true

    echo -e "${BOLD}Alga plugin status${NC}"
    echo ""

    if [ -d "$PLUGIN_DIR" ]; then
        local changed=0
        for f in "${PLUGIN_FILES[@]}"; do
            if ! check_file_sync "$SOURCE_DIR/$f" "$PLUGIN_DIR/$f"; then
                changed=$((changed + 1))
            fi
        done
        if [ "$changed" -eq 0 ]; then
            echo -e "  plugin files    ${GREEN}installed, up to date${NC}  ${DIM}$PLUGIN_DIR${NC}"
        else
            echo -e "  plugin files    ${YELLOW}installed, $changed file(s) outdated${NC}  (run install.sh to sync)"
            all_ok=false
        fi
    else
        echo -e "  plugin files    ${RED}not installed${NC}"
        all_ok=false
    fi

    if check_httpx; then
        echo -e "  httpx           ${GREEN}available${NC}"
    else
        echo -e "  httpx           ${RED}missing${NC}"
        all_ok=false
    fi

    if check_env_set; then
        local url
        url="$(grep '^ALGA_SERVER_URL=' "$HERMES_HOME/.env" | head -1 | cut -d= -f2-)"
        echo -e "  env vars        ${GREEN}configured${NC}  ${DIM}ALGA_SERVER_URL=$url${NC}"
    else
        echo -e "  env vars        ${YELLOW}not set${NC}  ${DIM}ALGA_SERVER_URL, ALGA_AGENT_TOKEN${NC}"
        all_ok=false
    fi

    if [ -f "$HERMES_HOME/config.yaml" ] && command -v grep &>/dev/null; then
        if grep -q "alga-platform" "$HERMES_HOME/config.yaml" 2>/dev/null; then
            echo -e "  plugin enabled  ${GREEN}yes${NC}"
        else
            if [ "$PROFILE" = "default" ]; then
                echo -e "  plugin enabled  ${YELLOW}no${NC}  ${DIM}run: hermes plugins enable $PLUGIN_NAME${NC}"
            else
                echo -e "  plugin enabled  ${YELLOW}no${NC}  ${DIM}run: hermes --profile $PROFILE plugins enable $PLUGIN_NAME${NC}"
            fi
            all_ok=false
        fi
        if grep -q '"alga"' "$HERMES_HOME/config.yaml" 2>/dev/null || grep -q '^- alga$' "$HERMES_HOME/config.yaml" 2>/dev/null; then
            echo -e "  toolset         ${GREEN}enabled${NC}"
        else
            if [ "$PROFILE" = "default" ]; then
                echo -e "  toolset         ${YELLOW}no${NC}  ${DIM}run: hermes tools enable alga${NC}"
            else
                echo -e "  toolset         ${YELLOW}no${NC}  ${DIM}run: hermes --profile $PROFILE tools enable alga${NC}"
            fi
            all_ok=false
        fi
    else
        echo -e "  plugin enabled  ${DIM}unknown${NC}"
        echo -e "  toolset         ${DIM}unknown${NC}"
        all_ok=false
    fi

    echo ""
    if $all_ok; then
        info "Everything is configured."
    else
        warn "Some items need attention (see above)."
    fi
}

# ── Uninstall ───────────────────────────────────────────────────────────

if [ "$ACTION" = "uninstall" ]; then
    if [ -d "$PLUGIN_DIR" ]; then
        rm -rf "$PLUGIN_DIR"
        info "Removed $PLUGIN_DIR"
    else
        warn "Plugin not installed at $PLUGIN_DIR"
    fi
    if [ "$PROFILE" = "default" ]; then
        info "Run 'hermes plugins list' to verify removal."
    else
        info "Run 'hermes --profile $PROFILE plugins list' to verify removal."
    fi
    exit 0
fi

# ── Status only ─────────────────────────────────────────────────────────

if [ "$ACTION" = "status" ]; then
    show_status
    exit 0
fi

# ── Preflight ───────────────────────────────────────────────────────────

[ -d "$HERMES_HOME" ] || die "Hermes home not found at $HERMES_HOME. Install Hermes Agent first."
[ -d "$SOURCE_DIR" ] || die "Plugin source not found at $SOURCE_DIR. Run from the alga-hermes-agent-plugin directory."

# ── Sync plugin files ───────────────────────────────────────────────────

mkdir -p "$PLUGIN_DIR"

changed=0
created=0
for f in "${PLUGIN_FILES[@]}"; do
    if [ ! -f "$PLUGIN_DIR/$f" ]; then
        cp "$SOURCE_DIR/$f" "$PLUGIN_DIR/$f"
        created=$((created + 1))
    elif ! check_file_sync "$SOURCE_DIR/$f" "$PLUGIN_DIR/$f"; then
        cp "$SOURCE_DIR/$f" "$PLUGIN_DIR/$f"
        changed=$((changed + 1))
    fi
done

if [ "$created" -gt 0 ]; then
    info "Installed plugin ($created file(s)) to $PLUGIN_DIR"
elif [ "$changed" -gt 0 ]; then
    info "Synced plugin ($changed file(s) updated)"
else
    info "Plugin files up to date"
fi

# ── Sync httpx ──────────────────────────────────────────────────────────

if ! check_httpx; then
    if [ -x "$HERMES_PYTHON" ]; then
        info "Installing httpx into Hermes venv..."
        "$HERMES_PIP" install 'httpx>=0.27' -q
    elif command -v python3 &>/dev/null; then
        warn "Hermes venv not found at $HERMES_VENV — installing httpx with system python3"
        pip3 install 'httpx>=0.27' -q 2>/dev/null || pip install 'httpx>=0.27' -q
    else
        warn "No python3 found. Install httpx manually: pip install 'httpx>=0.27'"
    fi
fi

# ── Enable plugin and toolset ───────────────────────────────────────────

info "Enabling Hermes plugin $PLUGIN_NAME..."
run_hermes plugins enable "$PLUGIN_NAME"

info "Enabling Alga toolset..."
run_hermes tools enable alga

# ── Show what still needs doing ─────────────────────────────────────────

needs=()

if ! check_env_set; then
    needs+=("env vars: add ALGA_SERVER_URL and ALGA_AGENT_TOKEN to $HERMES_HOME/.env")
fi

echo ""
warn "Remaining steps:"
for step in "${needs[@]}"; do
    echo -e "  ${YELLOW}-${NC} $step"
done
if [ "$PROFILE" = "default" ]; then
    echo -e "  ${YELLOW}-${NC} restart: hermes gateway restart"
else
    echo -e "  ${YELLOW}-${NC} restart: hermes --profile $PROFILE gateway restart"
fi
