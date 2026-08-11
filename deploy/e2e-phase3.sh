#!/usr/bin/env bash
# E2E manual de la Fase 3: api + worker + MailHog. Firma un POST /v1/notifications
# y comprueba que MailHog recibe el correo y la notificación queda SENT.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

GO="${GO:-go}"
COMPOSE="docker-compose -f deploy/docker-compose.yml"
export DATABASE_URL="postgres://qbnotify:qbnotify@localhost:55432/qbnotify?sslmode=disable"
export REDIS_SENTINELS="127.0.0.1:26379,127.0.0.1:26380,127.0.0.1:26381"
export REDIS_MASTER_NAME="master"
export SECRET_ENCRYPTION_KEY="AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
export ADMIN_JWT_SECRET="e2e"
export SEED_API_KEY_SECRET="demo-secret"
export API_PORT=8080

API_PID=""; WORKER_PID=""
cleanup() {
  [ -n "$API_PID" ] && kill "$API_PID" 2>/dev/null || true
  [ -n "$WORKER_PID" ] && kill "$WORKER_PID" 2>/dev/null || true
  $COMPOSE down -v >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "==> Infra + migrate + seed"
$COMPOSE up -d >/dev/null
for i in $(seq 1 30); do $COMPOSE exec -T postgres pg_isready -U qbnotify -p 55432 >/dev/null 2>&1 && break; sleep 1; done
(cd backend && $GO run ./cmd/migrate up >/dev/null)
(cd backend && $GO run ./cmd/seed >/dev/null)

echo "==> Arrancar api + worker"
(cd backend && $GO build -o ../bin/api ./cmd/api && $GO build -o ../bin/worker ./cmd/worker)
./bin/api >/tmp/api.log 2>&1 & API_PID=$!
./bin/worker >/tmp/worker.log 2>&1 & WORKER_PID=$!
sleep 3

echo "==> Firmar y enviar POST /v1/notifications"
METHOD=POST; PATHV="/v1/notifications"; TS=$(date +%s)
BODY='{"templateKey":"welcome","notificationType":"transactional","channel":"EMAIL","recipient":{"email":"jane@acme.test","name":"Jane"},"payload":{"firstName":"Jane","activationUrl":"https://acme.test/go","appName":"Acme","year":"2026"},"idempotencyKey":"e2e-1"}'
BODY_HASH=$(printf '%s' "$BODY" | openssl dgst -sha256 | awk '{print $NF}')
STRING_TO_SIGN=$(printf '%s\n%s\n%s\n%s' "$METHOD" "$PATHV" "$BODY_HASH" "$TS")
SIG=$(printf '%s' "$STRING_TO_SIGN" | openssl dgst -sha256 -hmac "demo-secret" | awk '{print $NF}')

RESP=$(curl -s -X POST "localhost:8080$PATHV" \
  -H "Content-Type: application/json" \
  -H "X-QBN-Key-Id: demo-key" \
  -H "X-QBN-Timestamp: $TS" \
  -H "X-QBN-Signature: $SIG" \
  -d "$BODY")
echo "    respuesta: $RESP"
NID=$(echo "$RESP" | grep -o '"id":"[^"]*"' | cut -d'"' -f4)
[ -z "$NID" ] && { echo "ERROR: no se obtuvo id. api.log:"; cat /tmp/api.log; exit 1; }

echo "==> Esperar procesamiento del worker"
STATUS=""
for i in $(seq 1 15); do
  STATUS=$($COMPOSE exec -T postgres psql -U qbnotify -p 55432 -d qbnotify -tAc "SELECT status FROM notifications WHERE id='$NID'")
  [ "$STATUS" = "SENT" ] && break
  sleep 1
done
echo "    estado final: $STATUS"

echo "==> Comprobar MailHog"
COUNT=$(curl -s "localhost:8025/api/v2/messages" | grep -o '"total":[0-9]*' | head -1 | cut -d: -f2)
echo "    mensajes en MailHog: $COUNT"

echo "==> Reintento idempotente (misma idempotencyKey → mismo id)"
TS2=$(date +%s)
SIG2=$(printf '%s\n%s\n%s\n%s' "$METHOD" "$PATHV" "$BODY_HASH" "$TS2" | openssl dgst -sha256 -hmac "demo-secret" | awk '{print $NF}')
RESP2=$(curl -s -X POST "localhost:8080$PATHV" -H "Content-Type: application/json" \
  -H "X-QBN-Key-Id: demo-key" -H "X-QBN-Timestamp: $TS2" -H "X-QBN-Signature: $SIG2" -d "$BODY")
NID2=$(echo "$RESP2" | grep -o '"id":"[^"]*"' | cut -d'"' -f4)
echo "    id1=$NID id2=$NID2"

if [ "$STATUS" = "SENT" ] && [ "${COUNT:-0}" -ge 1 ] && [ "$NID" = "$NID2" ]; then
  echo "==> E2E FASE 3 OK ✅"
else
  echo "==> E2E FALLÓ"; echo "--- worker.log ---"; tail -20 /tmp/worker.log; exit 1
fi
