#!/usr/bin/env bash
# Compose smoke (NFR-PORT-001): create a key, subscribe mock-subscriber,
# enqueue one event, and require a signed POST on the mock within ~5s.
#
# Prerequisite: docker compose up --build (api, worker, db, redis, mock-subscriber).
# This script never prints the API key, signing secret, or payload body.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

API_BASE="${COURIER_DEMO_API:-http://localhost:8080}"
MOCK_TARGET="http://mock-subscriber:9090/hooks"
DELIVER_DEADLINE_S="${COURIER_DEMO_DEADLINE_S:-5}"
BOOT_DEADLINE_S="${COURIER_DEMO_BOOT_S:-60}"

log() { printf 'demo: %s\n' "$*"; }
die() { printf 'demo: error: %s\n' "$*" >&2; exit 1; }

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "missing command: $1"
}

need_cmd curl
need_cmd docker
need_cmd python3

wait_http() {
  local url="$1"
  local deadline=$((SECONDS + BOOT_DEADLINE_S))
  while ((SECONDS < deadline)); do
    if curl -fsS --max-time 2 "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  die "timed out waiting for ${url}. Start the stack with: docker compose up --build"
}

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

mock_has_signed_post() {
  local event_id="$1"
  local logs
  logs="$(docker compose logs mock-subscriber --no-color 2>/dev/null || true)"
  printf '%s' "$logs" | python3 -c "
import json, sys
want = sys.argv[1]
found = False
for line in sys.stdin:
    line = line.strip()
    brace = line.find('{')
    if brace < 0:
        continue
    try:
        obj = json.loads(line[brace:])
    except json.JSONDecodeError:
        continue
    if obj.get('idempotency_key') != want:
        continue
    if obj.get('method', 'POST') != 'POST':
        continue
    if obj.get('has_signature') is True:
        found = True
        break
sys.exit(0 if found else 1)
" "$event_id"
}

log "waiting for ${API_BASE}/healthz and /readyz"
wait_http "${API_BASE}/healthz"
wait_http "${API_BASE}/readyz"
for svc in worker mock-subscriber; do
  if ! docker compose ps --status running --services 2>/dev/null | grep -qx "$svc"; then
    die "compose service ${svc} is not running. Start the stack with: docker compose up --build"
  fi
done

KEY_NAME="demo-$(date +%s)-$$"
log "creating API key ${KEY_NAME}"
KEY_OUT="$(docker compose run --rm --no-deps -T api keys create --name "$KEY_NAME" 2>/dev/null)" || die "keys create failed (is Compose up?)"
API_KEY="$(printf '%s\n' "$KEY_OUT" | awk '/^  key:/{print $2; exit}')"
[[ -n "$API_KEY" ]] || die "could not parse API key from keys create output"
log "API key created (value not printed)"

log "creating subscription to mock-subscriber"
http_json POST /v1/subscriptions "$API_KEY" \
  "{\"target_url\":\"${MOCK_TARGET}\",\"event_types\":[\"*\"],\"description\":\"compose demo\"}"
if [[ "$HTTP_STATUS" != "201" ]]; then
  die "create subscription expected 201, got ${HTTP_STATUS}"
fi
SUB_ID="$(json_get "$HTTP_BODY" "id")"
[[ -n "$SUB_ID" ]] || die "subscription response missing id"
log "subscription created id=${SUB_ID}"

IDEM="demo-${KEY_NAME}"
log "enqueueing order.created"
http_json POST /v1/events "$API_KEY" \
  "{\"type\":\"order.created\",\"payload\":{\"source\":\"scripts/demo.sh\"},\"idempotency_key\":\"${IDEM}\"}"
if [[ "$HTTP_STATUS" != "201" ]]; then
  die "enqueue expected 201, got ${HTTP_STATUS}"
fi
EVENT_ID="$(json_get "$HTTP_BODY" "event_id")"
[[ -n "$EVENT_ID" ]] || die "enqueue response missing event_id"
log "enqueued event_id=${EVENT_ID}; waiting up to ${DELIVER_DEADLINE_S}s for signed POST"

deadline=$((SECONDS + DELIVER_DEADLINE_S))
while ((SECONDS < deadline)); do
  if mock_has_signed_post "$EVENT_ID"; then
    log "ok: mock-subscriber received signed POST (X-Courier-Idempotency-Key=${EVENT_ID})"
    exit 0
  fi
  sleep 0.2
done

die "mock-subscriber did not receive a signed POST for event ${EVENT_ID} within ${DELIVER_DEADLINE_S}s"
