#!/usr/bin/env bash
# Seed script for Alga — creates dummy teams, services, escalation policies,
# on-call schedules, and test users for production-like testing.
#
# Prerequisites:
#   - Alga backend running (default http://localhost:8080)
#   - curl, jq installed
#
# Usage:
#   ALGA_URL=http://localhost:8080 bash scripts/seed.sh

set -euo pipefail

BASE="${ALGA_URL:-http://localhost:8080}"
COOKIE_JAR=$(mktemp)
trap 'rm -f "$COOKIE_JAR"' EXIT

ADMIN_EMAIL="${ALGA_ADMIN_EMAIL:-admin@alga.local}"
if [ -z "${ALGA_ADMIN_PASSWORD:-}" ]; then
  echo "ERROR: ALGA_ADMIN_PASSWORD is not set." >&2
  echo "       Create the initial admin via the setup wizard (/setup) on first boot," >&2
  echo "       then set ALGA_ADMIN_PASSWORD to that password before running this script." >&2
  exit 1
fi
ADMIN_PASSWORD="${ALGA_ADMIN_PASSWORD}"

CSRF=""

api() {
  local method="$1" path="$2"
  shift 2
  curl -sS -b "$COOKIE_JAR" -c "$COOKIE_JAR" \
    -H "Content-Type: application/json" \
    -H "X-CSRF-Token: $CSRF" \
    -X "$method" \
    "${BASE}${path}" \
    "$@"
}

login() {
  echo "==> Logging in as ${ADMIN_EMAIL}"
  local resp
  resp=$(api POST /api/v1/auth/login -d "{\"email\":\"${ADMIN_EMAIL}\",\"password\":\"${ADMIN_PASSWORD}\"}")
  CSRF=$(echo "$resp" | jq -r '.csrf_token // empty')
  if [ -z "$CSRF" ]; then
    echo "ERROR: login failed — $resp" >&2
    exit 1
  fi
  echo "    Logged in. CSRF token acquired."
}

create_user() {
  local email="$1" password="$2" role="$3" full_name="$4"
  local resp
  resp=$(api POST /api/v1/users -d "$(jq -n \
    --arg e "$email" --arg p "$password" --arg r "$role" --arg n "$full_name" \
    '{email:$e, password:$p, role:$r, full_name:$n}')")
  local id
  id=$(echo "$resp" | jq -r '.id // empty')
  if [ -z "$id" ]; then
    echo "    WARN: could not create user $email — $resp" >&2
    echo ""
  else
    echo "$id"
  fi
}

create_team() {
  local name="$1" desc="${2:-}"
  local resp
  resp=$(api POST /api/v1/teams -d "$(jq -n \
    --arg n "$name" --arg d "$desc" \
    '{name:$n, description:$d}')")
  local id
  id=$(echo "$resp" | jq -r '.id // empty')
  if [ -z "$id" ]; then
    echo "    WARN: could not create team $name — $resp" >&2
    echo ""
  else
    echo "$id"
  fi
}

add_team_member() {
  local team_id="$1" user_id="$2" role="${3:-member}"
  api POST "/api/v1/teams/${team_id}/members" -d "$(jq -n \
    --arg u "$user_id" --arg r "$role" \
    '{user_id:$u, role:$r}')" > /dev/null
}

create_service() {
  local name="$1" display_name="$2" desc="$3" owner_team_id="${4:-}" \
        sla_response="${5:-15}" sla_resolve="${6:-60}"
  local resp
  local extra=""
  if [ -n "$owner_team_id" ]; then
    extra=", \"owner_team_id\": \"${owner_team_id}\""
  fi
  resp=$(api POST /api/v1/services -d "$(jq -n \
    --arg n "$name" --arg dn "$display_name" --arg d "$desc" \
    --argjson sr "$sla_response" --argjson sl "$sla_resolve" \
    "{name:\$n, display_name:\$dn, description:\$d, \
      sla_response_minutes:\$sr, sla_resolve_minutes:\$sl ${extra}}")")
  local id
  id=$(echo "$resp" | jq -r '.id // empty')
  if [ -z "$id" ]; then
    echo "    WARN: could not create service $name — $resp" >&2
    echo ""
  else
    echo "$id"
  fi
}

create_escalation_policy() {
  local name="$1" desc="$2" repeat="${3:-0}" levels_json="$4"
  local resp
  resp=$(api POST /api/v1/escalation-policies -d "$(jq -n \
    --arg n "$name" --arg d "$desc" --argjson r "$repeat" \
    --argjson l "$levels_json" \
    '{name:$n, description:$d, repeat_count:$r, levels:$l}')")
  local id
  id=$(echo "$resp" | jq -r '.id // empty')
  if [ -z "$id" ]; then
    echo "    WARN: could not create escalation policy $name — $resp" >&2
    echo ""
  else
    echo "$id"
  fi
}

create_schedule() {
  local name="$1" desc="$2" tz="$3" team_id="${4:-}" layers_json="$5"
  local extra=""
  if [ -n "$team_id" ]; then
    extra=", \"team_id\": \"${team_id}\""
  fi
  local resp
  resp=$(api POST /api/v1/on-call/schedules -d "$(jq -n \
    --arg n "$name" --arg d "$desc" --arg tz "$tz" \
    --argjson l "$layers_json" \
    "{name:\$n, description:\$d, timezone:\$tz, layers:\$l ${extra}}")")
  local id
  id=$(echo "$resp" |jq -r '.id // empty')
  if [ -z "$id" ]; then
    echo "    WARN: could not create schedule $name — $resp" >&2
    echo ""
  else
    echo "$id"
  fi
}

create_override() {
  local schedule_id="$1" user_id="$2" start="$3" end="$4"
  api POST "/api/v1/on-call/schedules/${schedule_id}/overrides" -d "$(jq -n \
    --arg u "$user_id" --arg s "$start" --arg e "$end" \
    '{user_id:$u, start_at:$s, end_at:$e}')" > /dev/null
}

# ─────────────────────────────────────────────────────────────────────────────
# Main
# ─────────────────────────────────────────────────────────────────────────────

login

# ── Admin (logged-in user) ────────────────────────────────────────────────────
echo ""
echo "==> Resolving admin user"
ADMIN_ID=$(api GET /api/v1/auth/me | jq -r '.id // empty')
if [ -n "$ADMIN_ID" ]; then
  echo "    admin → ${ADMIN_ID}"
else
  echo "    WARN: could not resolve admin user id" >&2
fi

# ── Users ─────────────────────────────────────────────────────────────────────
echo ""
echo "==> Creating test users"

USER_ALICE=$(create_user "alice@alga.local" "Op3r@tor!" "operator" "Alice Chen")
echo "    alice@alga.local → ${USER_ALICE:-SKIPPED}"

USER_BOB=$(create_user "bob@alga.local" "V1ewer!pass" "viewer" "Bob Martinez")
echo "    bob@alga.local   → ${USER_BOB:-SKIPPED}"

USER_CAROL=$(create_user "carol@alga.local" "Op3r@tor2" "operator" "Carol Nguyen")
echo "    carol@alga.local → ${USER_CAROL:-SKIPPED}"

USER_DAVE=$(create_user "dave@alga.local" "Adm1n!pass" "admin" "Dave Park")
echo "    dave@alga.local  → ${USER_DAVE:-SKIPPED}"

# ── Teams ─────────────────────────────────────────────────────────────────────
echo ""
echo "==> Creating teams"

TEAM_ENG=$(create_team "Engineering" "Platform and backend engineering")
echo "    Engineering  → ${TEAM_ENG:-SKIPPED}"

TEAM_SRE=$(create_team "SRE" "Site reliability and on-call")
echo "    SRE          → ${TEAM_SRE:-SKIPPED}"

TEAM_PAYMENTS=$(create_team "Payments" "Payment processing and billing")
echo "    Payments     → ${TEAM_PAYMENTS:-SKIPPED}"

TEAM_OPS=$(create_team "Ops" "Operations and incident response")
echo "    Ops          → ${TEAM_OPS:-SKIPPED}"

# ── Team members ──────────────────────────────────────────────────────────────
echo ""
echo "==> Adding team members"

if [ -n "$TEAM_ENG" ]; then
  [ -n "$USER_ALICE" ] && add_team_member "$TEAM_ENG" "$USER_ALICE" "lead" && echo "    Alice → Engineering (lead)"
  [ -n "$USER_CAROL" ] && add_team_member "$TEAM_ENG" "$USER_CAROL" && echo "    Carol → Engineering"
fi

if [ -n "$TEAM_SRE" ]; then
  [ -n "$USER_DAVE" ] && add_team_member "$TEAM_SRE" "$USER_DAVE" "lead" && echo "    Dave  → SRE (lead)"
  [ -n "$USER_ALICE" ] && add_team_member "$TEAM_SRE" "$USER_ALICE" && echo "    Alice → SRE"
  [ -n "$USER_CAROL" ] && add_team_member "$TEAM_SRE" "$USER_CAROL" && echo "    Carol → SRE"
fi

if [ -n "$TEAM_PAYMENTS" ]; then
  [ -n "$USER_BOB" ] && add_team_member "$TEAM_PAYMENTS" "$USER_BOB" "lead" && echo "    Bob   → Payments (lead)"
  [ -n "$USER_CAROL" ] && add_team_member "$TEAM_PAYMENTS" "$USER_CAROL" && echo "    Carol → Payments"
fi

if [ -n "$TEAM_OPS" ] && [ -n "$ADMIN_ID" ]; then
  add_team_member "$TEAM_OPS" "$ADMIN_ID" "lead" && echo "    Admin → Ops (lead)"
fi

# ── Escalation policies ──────────────────────────────────────────────────────
echo ""
echo "==> Creating escalation policies"

LEVELS_DEFAULT=$(jq -n \
  --arg uid "${USER_ALICE}" --arg tid "${TEAM_SRE}" \
  '[
    {level_number:1, delay_minutes:15, notify_channels:["email","slack"],
     targets:[{target_type:"user", target_user_id:$uid}]},
    {level_number:2, delay_minutes:30, notify_channels:["email","slack"],
     targets:[{target_type:"team", target_team_id:$tid}]}
  ]')
EP_DEFAULT=$(create_escalation_policy "Default Escalation" \
  "Standard two-tier escalation" 2 "$LEVELS_DEFAULT")
echo "    Default Escalation → ${EP_DEFAULT:-SKIPPED}"

LEVELS_CRITICAL=$(jq -n \
  --arg uid "${USER_DAVE}" --arg tid "${TEAM_SRE}" \
  '[
    {level_number:1, delay_minutes:5, notify_channels:["email","slack","voice"],
     targets:[{target_type:"user", target_user_id:$uid}]},
    {level_number:2, delay_minutes:15, notify_channels:["email","slack"],
     targets:[{target_type:"team", target_team_id:$tid}]}
  ]')
EP_CRITICAL=$(create_escalation_policy "Critical Escalation" \
  "Immediate escalation for critical severity" 3 "$LEVELS_CRITICAL")
echo "    Critical Escalation → ${EP_CRITICAL:-SKIPPED}"

# ── Services ──────────────────────────────────────────────────────────────────
echo ""
echo "==> Creating services"

SVC_API=$(create_service "api-gateway" "API Gateway" \
  "Main API gateway handling all external traffic" \
  "$TEAM_ENG" 10 30)
echo "    api-gateway  → ${SVC_API:-SKIPPED}"

SVC_PAYMENTS=$(create_service "payment-service" "Payment Service" \
  "Handles payment processing, refunds, and billing" \
  "$TEAM_PAYMENTS" 15 60)
echo "    payment-service → ${SVC_PAYMENTS:-SKIPPED}"

SVC_AUTH=$(create_service "auth-service" "Authentication Service" \
  "OAuth2/OIDC provider and session management" \
  "$TEAM_ENG" 10 45)
echo "    auth-service → ${SVC_AUTH:-SKIPPED}"

SVC_NOTIFICATIONS=$(create_service "notification-service" "Notification Service" \
  "Push, email, and in-app notification delivery" \
  "$TEAM_ENG" 15 60)
echo "    notification-service → ${SVC_NOTIFICATIONS:-SKIPPED}"

SVC_DB=$(create_service "database-primary" "Database Primary" \
  "Primary PostgreSQL cluster" \
  "$TEAM_SRE" 5 30)
echo "    database-primary → ${SVC_DB:-SKIPPED}"

# ── On-call schedules ─────────────────────────────────────────────────────────
echo ""
echo "==> Creating on-call schedules"

SCHED_PRIMARY=$(create_schedule "Primary On-Call" \
  "Weekly rotation for primary on-call coverage" \
  "UTC" "$TEAM_SRE" \
  "$(jq -n '[]' | jq \
    --arg uid1 "${USER_ALICE}" --arg uid2 "${USER_CAROL}" --arg uid3 "${USER_DAVE}" \
    --arg start "$(date -u +%Y-%m-%dT00:00:00Z)" \
    '. += [
      {name:"Weekday Rotation", rotation_type:"weekly", rotation_interval:1,
       start_date:$start, user_ids:[$uid1,$uid2,$uid3],
       start_time:"09:00", end_time:"17:00",
       days_of_week:["Monday","Tuesday","Wednesday","Thursday","Friday"], priority:1}
    ]')")
echo "    Primary On-Call → ${SCHED_PRIMARY:-SKIPPED}"

SCHED_WEEKEND=$(create_schedule "Weekend On-Call" \
  "Weekend rotation with reduced coverage" \
  "UTC" "$TEAM_SRE" \
  "$(jq -n '[]' | jq \
    --arg uid1 "${USER_DAVE}" --arg uid2 "${USER_CAROL}" \
    --arg start "$(date -u +%Y-%m-%dT00:00:00Z)" \
    '. += [
      {name:"Weekend Rotation", rotation_type:"weekly", rotation_interval:1,
       start_date:$start, user_ids:[$uid1,$uid2],
       start_time:"10:00", end_time:"16:00",
       days_of_week:["Saturday","Sunday"], priority:0}
    ]')")
echo "    Weekend On-Call → ${SCHED_WEEKEND:-SKIPPED}"

SCHED_PAYMENTS=$(create_schedule "Payments On-Call" \
  "On-call rotation for the payments team" \
  "America/New_York" "$TEAM_PAYMENTS" \
  "$(jq -n '[]' | jq \
    --arg uid1 "${USER_BOB}" --arg uid2 "${USER_CAROL}" \
    --arg start "$(date -u +%Y-%m-%dT00:00:00Z)" \
    '. += [
      {name:"Payments Weekly", rotation_type:"weekly", rotation_interval:1,
       start_date:$start, user_ids:[$uid1,$uid2]}
    ]')")
echo "    Payments On-Call → ${SCHED_PAYMENTS:-SKIPPED}"

# ── Override (example) ────────────────────────────────────────────────────────
echo ""
echo "==> Creating schedule override"

NEXT_MONDAY=$(date -u -d "next monday +%Y-%m-%dT09:00:00Z" 2>/dev/null || date -u -v+monday +%Y-%m-%dT09:00:00Z 2>/dev/null || echo "")
NEXT_MONDAY_END=$(date -u -d "next monday +%Y-%m-%dT17:00:00Z" 2>/dev/null || date -u -v+monday +%Y-%m-%dT17:00:00Z 2>/dev/null || echo "")

if [ -n "$SCHED_PRIMARY" ] && [ -n "$USER_DAVE" ] && [ -n "$NEXT_MONDAY" ]; then
  create_override "$SCHED_PRIMARY" "$USER_DAVE" "$NEXT_MONDAY" "$NEXT_MONDAY_END"
  echo "    Dave overrides Primary On-Call next Monday"
fi

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo "==> Seed complete"
echo "    Users:                4"
echo "    Teams:                4"
echo "    Services:             5"
echo "    Escalation policies:  2"
echo "    On-call schedules:    3"
echo "    Overrides:            1"
echo ""
echo "    Login emails:  alice@alga.local  bob@alga.local  carol@alga.local  dave@alga.local"
