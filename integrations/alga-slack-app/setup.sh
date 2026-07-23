#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MANIFEST_FILE="$SCRIPT_DIR/manifest.json"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info()  { echo -e "${BLUE}[INFO]${NC}  $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }

check_deps() {
    local missing=()
    for cmd in curl jq; do
        if ! command -v "$cmd" &>/dev/null; then
            missing+=("$cmd")
        fi
    done
    if [ ${#missing[@]} -gt 0 ]; then
        error "Missing required tools: ${missing[*]}"
        echo "Install them before running this script."
        exit 1
    fi
}

validate_manifest() {
    if [ ! -f "$MANIFEST_FILE" ]; then
        error "manifest.json not found at $MANIFEST_FILE"
        exit 1
    fi

    if grep -q "YOUR_ALGA_HOST" "$MANIFEST_FILE"; then
        error "manifest.json still contains placeholder 'YOUR_ALGA_HOST'."
        echo ""
        echo "Replace all occurrences of 'YOUR_ALGA_HOST' with your Alga instance URL."
        echo "For example:"
        echo "  sed -i 's|YOUR_ALGA_HOST|alga.example.com|g' $MANIFEST_FILE"
        echo ""
        echo "Then re-run this script."
        exit 1
    fi

    if ! jq empty "$MANIFEST_FILE" 2>/dev/null; then
        error "manifest.json is not valid JSON."
        exit 1
    fi

    ok "manifest.json is valid and has no placeholders."
}

show_manual_setup() {
    echo ""
    echo "========================================="
    echo "  Alga Slack App — Manual Setup Guide"
    echo "========================================="
    echo ""
    echo "1. Go to https://api.slack.com/apps and click 'Create New App'."
    echo "2. Select 'From a manifest'."
    echo "3. Choose your workspace."
    echo "4. Paste the contents of manifest.json (or upload the file)."
    echo ""
    echo "  manifest.json location:"
    echo "  $MANIFEST_FILE"
    echo ""
    echo "5. After creating the app, go to:"
    echo "   Basic Information → App Credentials"
    echo ""
    echo "   Copy the following values:"
    echo "   • Signing Secret  → SLACK_SIGNING_SECRET"
    echo "   • Bot User OAuth Token (under OAuth & Permissions) → SLACK_BOT_TOKEN"
    echo ""
    echo "6. Set these environment variables for your Alga backend:"
    echo ""
    echo "   export SLACK_SIGNING_SECRET=\"<your-signing-secret>\""
    echo "   export SLACK_BOT_TOKEN=\"<your-bot-token>\""
    echo ""
    echo "7. Install the app to your workspace:"
    echo "   OAuth & Permissions → Install to Workspace → Allow"
    echo ""
    echo "8. Verify the endpoint:"
    echo "   curl -s https://<your-alga-host>/health"
    echo ""
    ok "Manual setup instructions displayed."
}

show_interactive_setup() {
    echo ""
    echo "========================================="
    echo "  Alga Slack App — Interactive Setup"
    echo "========================================="
    echo ""

    if [ -z "${SLACK_TOKEN:-}" ]; then
        warn "SLACK_TOKEN environment variable not set."
        echo ""
        echo "To create the app programmatically, you need a Slack workspace admin token."
        echo "Set it with:"
        echo "  export SLACK_TOKEN=\"xoxp-...\""
        echo ""
        echo "Falling back to manual setup."
        show_manual_setup
        return
    fi

    local app_name
    app_name=$(jq -r '.display_information.name' "$MANIFEST_FILE")

    info "Creating Slack app '$app_name' from manifest..."

    local manifest_json
    manifest_json=$(cat "$MANIFEST_FILE")

    local response
    response=$(curl -s -X POST "https://slack.com/api/apps.manifest.create" \
        -H "Authorization: Bearer $SLACK_TOKEN" \
        -H "Content-Type: application/json" \
        -d "{\"manifest\":\"$(echo "$manifest_json" | jq -Rs .)\",\"team_id\":\"${SLACK_TEAM_ID:-}\"}")

    local app_ok
    app_ok=$(echo "$response" | jq -r '.ok')

    if [ "$app_ok" != "true" ]; then
        local err_msg
        err_msg=$(echo "$response" | jq -r '.error // "unknown error"')
        error "Failed to create Slack app: $err_msg"
        echo ""
        echo "Full response:"
        echo "$response" | jq .
        echo ""
        echo "Falling back to manual setup."
        show_manual_setup
        return
    fi

    local app_id
    app_id=$(echo "$response" | jq -r '.app_id // "unknown"')

    ok "Slack app created successfully! App ID: $app_id"
    echo ""
    echo "Next steps:"
    echo "1. Go to https://api.slack.com/apps/$app_id"
    echo "2. Navigate to 'OAuth & Permissions' → 'Install to Workspace'"
    echo "3. Copy the 'Bot User OAuth Token' (starts with xoxb-)"
    echo "4. Copy the 'Signing Secret' from 'Basic Information'"
    echo "5. Configure Alga:"
    echo "   export SLACK_SIGNING_SECRET=\"<signing-secret>\""
    echo "   export SLACK_BOT_TOKEN=\"<bot-token>\""
}

main() {
    echo ""
    echo "Alga Slack App Setup"
    echo "===================="
    echo ""

    check_deps
    validate_manifest

    if [ "${1:-}" = "--manual" ] || [ "${1:-}" = "-m" ]; then
        show_manual_setup
    elif [ "${1:-}" = "--interactive" ] || [ "${1:-}" = "-i" ]; then
        show_interactive_setup
    else
        echo "Usage: $0 [--manual|-m|--interactive|-i]"
        echo ""
        echo "Options:"
        echo "  --manual, -m        Show manual setup instructions (default)"
        echo "  --interactive, -i   Create the app via Slack API (requires SLACK_TOKEN)"
        echo ""
        show_manual_setup
    fi
}

main "$@"
