#!/usr/bin/env bash
# Smoke test de la Fase 0: levanta la infraestructura, arranca el API y verifica
# que /readyz responde sano (DB + Redis conectados). Se usa en local (`make
# smoke`) y en CI. Es autocontenido: define el entorno mínimo necesario.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

GO="${GO:-go}"
API_PORT="${API_PORT:-8080}"

# Detecta el comando de compose disponible: plugin (`docker compose`, típico en
# CI) o binario standalone (`docker-compose`, instalado en local).
if docker compose version >/dev/null 2>&1; then
  COMPOSE="docker compose -f deploy/docker-compose.yml"
else
  COMPOSE="docker-compose -f deploy/docker-compose.yml"
fi

# Entorno mínimo (casa con deploy/docker-compose.yml). Sobrescribible por env.
export APP_ENV="${APP_ENV:-development}"
export API_PORT
export DATABASE_URL="${DATABASE_URL:-postgres://qbnotify:qbnotify@localhost:55432/qbnotify?sslmode=disable}"
export REDIS_SENTINELS="${REDIS_SENTINELS:-127.0.0.1:26379,127.0.0.1:26380,127.0.0.1:26381}"
export REDIS_MASTER_NAME="${REDIS_MASTER_NAME:-master}"
export SECRET_ENCRYPTION_KEY="${SECRET_ENCRYPTION_KEY:-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=}"
export ADMIN_JWT_SECRET="${ADMIN_JWT_SECRET:-smoke-test-secret-de-32-bytes-o-mas-1234}"

API_PID=""
cleanup() {
  [ -n "$API_PID" ] && kill "$API_PID" 2>/dev/null || true
  $COMPOSE down -v >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "==> Levantando infraestructura..."
$COMPOSE up -d

echo "==> Esperando a Postgres y Redis..."
for i in $(seq 1 30); do
  if $COMPOSE exec -T postgres pg_isready -U qbnotify -p 55432 >/dev/null 2>&1 \
     && $COMPOSE exec -T redis-master redis-cli -p 6379 ping >/dev/null 2>&1; then
    echo "    infra lista"
    break
  fi
  sleep 2
  [ "$i" = "30" ] && { echo "ERROR: infra no quedó lista a tiempo"; exit 1; }
done

echo "==> Compilando y arrancando el API..."
(cd backend && $GO build -o ../bin/api ./cmd/api)
./bin/api &
API_PID=$!

echo "==> Sondeando /readyz..."
for i in $(seq 1 30); do
  code=$(curl -s -o /tmp/readyz.json -w '%{http_code}' "http://localhost:${API_PORT}/readyz" || echo "000")
  if [ "$code" = "200" ]; then
    echo "    /readyz OK (200):"
    cat /tmp/readyz.json; echo
    echo "==> SMOKE TEST OK"
    exit 0
  fi
  sleep 2
done

echo "ERROR: /readyz no respondió 200 (último código: ${code})"
cat /tmp/readyz.json 2>/dev/null || true
exit 1
