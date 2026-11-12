#!/usr/bin/env bash
# Enqueue N events and print enqueue throughput (NFR-PERF-002).
#
# Prerequisite: API is up (`docker compose up --build` or `courier serve`).
# Usage:
#   ./scripts/load.sh                  # COUNT=100, creates a key via Compose
#   COUNT=50 ./scripts/load.sh
#   KEY=co_... TARGET_URL=http://mock-subscriber:9090/hooks ./scripts/load.sh
#
# This script never prints the API key, signing secret, or payload body.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

API_BASE="${COURIER_LOAD_API:-http://localhost:8080}"
TARGET_URL="${TARGET_URL:-http://mock-subscriber:9090/hooks}"
COUNT="${COUNT:-100}"
EVENT_TYPE="${EVENT_TYPE:-load.test}"

log() { printf 'load: %s\n' "$*"; }
die() { printf 'load: error: %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<'EOF'
Usage: scripts/load.sh

Enqueue COUNT events and print enqueue rps.

Environment:
  COUNT             Number of events (default 100)
  KEY               Existing API key (if unset, created via docker compose)
  TARGET_URL        Subscription URL (default http://mock-subscriber:9090/hooks)
  EVENT_TYPE        Event type (default load.test)
  COURIER_LOAD_API  API base URL (default http://localhost:8080)
EOF
}

if [[ "${1-}" == "-h" || "${1-}" == "--help" ]]; then
  usage
  exit 0
fi

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "missing command: $1"
}

need_cmd curl
need_cmd python3
if ! [[ "$COUNT" =~ ^[1-9][0-9]*$ ]]; then
  die "COUNT must be a positive integer, got ${COUNT}"
fi

json_get() {
  local raw="$1"
  local expr="$2"
  printf '%s' "$raw" | python3 -c "
import json, sys
expr = sys.argv[1]
data = json.load(sys.stdin)
for part in expr.split('.'):
    if isinstance(data, dict):
        data = data[part]
    else:
        raise SystemExit('json path missed: ' + expr)
if data is None:
    raise SystemExit('missing ' + expr)
print(data)
" "$expr"
}

http_json() {
  local method="$1"
  local path="$2"
  local key="$3"
  local body="${4-}"
  local tmp status
  tmp="$(mktemp)"
  if [[ -n "$body" ]]; then
    status="$(curl -sS --max-time 10 -o "$tmp" -w '%{http_code}' \
      -X "$method" "${API_BASE}${path}" \
      -H "Authorization: Bearer ${key}" \
      -H 'Content-Type: application/json' \
      --data-binary "$body")"
  else
    status="$(curl -sS --max-time 10 -o "$tmp" -w '%{http_code}' \
      -X "$method" "${API_BASE}${path}" \
      -H "Authorization: Bearer ${key}")"
  fi
  HTTP_BODY="$(cat "$tmp")"
  rm -f "$tmp"
  HTTP_STATUS="$status"
}

if [[ -z "${KEY-}" ]]; then
  need_cmd docker
  KEY_NAME="load-$(date +%s)-$$"
  log "creating API key ${KEY_NAME}"
  KEY_OUT="$(docker compose run --rm --no-deps -T api keys create --name "$KEY_NAME" 2>/dev/null)" || die "keys create failed (is Compose up?)"
  KEY="$(printf '%s\n' "$KEY_OUT" | awk '/^  key:/{print $2; exit}')"
  [[ -n "$KEY" ]] || die "could not parse API key from keys create output"
  log "API key created (value not printed)"
fi

log "ensuring subscription to ${TARGET_URL}"
http_json POST /v1/subscriptions "$KEY" \
  "{\"target_url\":\"${TARGET_URL}\",\"event_types\":[\"${EVENT_TYPE}\"],\"description\":\"load script\"}"
if [[ "$HTTP_STATUS" != "201" && "$HTTP_STATUS" != "200" ]]; then
  die "create subscription expected 201, got ${HTTP_STATUS}"
fi

log "enqueueing ${COUNT} ${EVENT_TYPE} events"
start_ns="$(date +%s%N)"
ok=0
for i in $(seq 1 "$COUNT"); do
  http_json POST /v1/events "$KEY" \
    "{\"type\":\"${EVENT_TYPE}\",\"payload\":{\"n\":${i}},\"idempotency_key\":\"load-${start_ns}-${i}\"}"
  if [[ "$HTTP_STATUS" != "201" && "$HTTP_STATUS" != "200" ]]; then
    die "enqueue ${i} expected 201/200, got ${HTTP_STATUS}"
  fi
  ok=$((ok + 1))
done
end_ns="$(date +%s%N)"
elapsed_ns=$((end_ns - start_ns))
python3 -c "
elapsed = ${elapsed_ns} / 1e9
count = ${ok}
rps = count / elapsed if elapsed > 0 else 0
print('load: enqueued %d events in %.3fs (%.1f enqueue/s)' % (count, elapsed, rps))
"
