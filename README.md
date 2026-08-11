# QB Notify

Servicio de notificaciones multi-tenant (email, SMS, push, in-app) con colas, reintentos, plantillas y un panel de administración. Cada backend que consume el servicio es un tenant (**Application**) independiente, con sus propias API keys, plantillas, rutas de envío y cuotas.

> Este es el **v2**, reescrito en Go + React. El v1 (Node/Prisma) fue retirado por completo; no hay migración de datos entre ambos.

## Stack

- **Backend:** Go (`backend/`) — [chi](https://github.com/go-chi/chi) para HTTP, [asynq](https://github.com/hibiken/asynq) + Redis Sentinel para colas, PostgreSQL vía `pgx`/`sqlc`, migraciones con `goose`.
- **Frontend:** React + TS (`frontend/`) — Vite, Tailwind v4, shadcn/ui.
- **Infra:** Postgres, Redis (Sentinel), MailHog para SMTP en local.

## Estructura del repo

```
qb-notify/
├── backend/       # Go: cmd/ (api, worker, migrate, seed, reencrypt), internal/, db/ (migraciones + queries)
├── frontend/      # SPA de administración (Vite + React + TS + Tailwind + shadcn/ui)
├── deploy/        # Dockerfiles, docker-compose de infra local, .env.*.example, scripts de smoke/e2e
├── docs/          # documentación de progreso, guías de integración, openapi.yaml
├── new-version.md # documento de diseño original (referencia de intención)
└── CLAUDE.md      # guía de arquitectura y convenciones para trabajar en el repo
```

## Requisitos

- Go 1.26+ (`~/sdk/go/bin` en `PATH`, o el que tengas instalado)
- Node.js 20+
- Docker + Docker Compose (infra local)

## Arranque rápido

```bash
cp .env.example .env        # completa los valores (ver CLAUDE.md § Config & environment)

make up                      # Postgres + Redis Sentinel + MailHog
make migrate-up && make seed  # esquema + datos base (plantillas, super-admin, app demo)

make run-api                 # terminal 1 — :8080
make run-worker               # terminal 2 — asynq consumer + métricas :9091

cd frontend && npm install && npm run dev   # terminal 3 — SPA en :5173
```

El Makefile ejecuta los targets de Go dentro de `backend/` automáticamente (correr `make` desde la raíz del repo). Ver `make help` para todos los comandos disponibles (build, test, lint, migraciones, sqlc, smoke test, etc).

## Autenticación

- **API pública (`/v1`):** HMAC-SHA256 por request, con API keys por Application (`X-QBN-Key-Id` / `X-QBN-Timestamp` / `X-QBN-Signature`). Pensada para integraciones backend-a-backend.
- **Panel admin (`/admin`):** email + password → access token de corta duración (Bearer) + refresh token en cookie HttpOnly. Roles `SUPER_ADMIN` (todas las apps) y `APP_ADMIN` (una app).

Detalles completos de firmas, scopes y flujos en [`docs/API-INTEGRACION.md`](docs/API-INTEGRACION.md) y [`docs/GUIA-CONEXION-APP.md`](docs/GUIA-CONEXION-APP.md).

## Despliegue

Cada app (`backend`, `services`, `frontend`) tiene su propio Dockerfile en `deploy/` y un `.env.*.example` con las variables necesarias. La guía de despliegue en Dokploy está en [`deploy/deploy-staging.md`](deploy/deploy-staging.md). Los tres Dockerfiles se construyen **desde la raíz del repo** (necesitan `backend/`, `frontend/` y `deploy/` en el contexto):

```bash
docker build -f deploy/Dockerfile.backend  -t notify-backend  .
docker build -f deploy/Dockerfile.services -t notify-services .
docker build -f deploy/Dockerfile.frontend -t notify-frontend --build-arg VITE_API_URL=https://api.tu-dominio.com .
```

Las imágenes de producción se compilan y publican automáticamente a `docker.divergtech.com` vía `.github/workflows/{backend,services,frontend}.yml` en cada push a `main` (mismo patrón que otros proyectos de la org).

### `VITE_API_URL` en GitHub Actions (build-time, no runtime)

La URL pública del api se **hornea en el bundle** del frontend al compilar: Vite solo expone `import.meta.env.VITE_*` durante el build, así que **no** es una variable de entorno del contenedor — cambiarla en runtime (o reiniciar el contenedor) no tiene ningún efecto. Por eso `frontend.yml` la pasa como `build-arg` al Dockerfile, y hay que configurarla en GitHub antes del primer push a `main`:

1. En el repo: **Settings → Secrets and variables → Actions**.
2. Pestaña **Variables** → **New repository variable** (va como variable y no como secret: el workflow la lee con `${{ vars.VITE_API_URL }}` y no es un dato sensible — es la URL pública que cualquiera ve en el bundle).
3. Nombre: `VITE_API_URL` · Valor: la URL pública del backend, p. ej. `https://api.tu-dominio.com` (sin `/` final).

En esa misma página, pestaña **Secrets**, deben existir `REGISTRY_USERNAME` y `REGISTRY_PASSWORD` (login al registry `docker.divergtech.com`); esos sí son secrets porque son credenciales. Para cambiar la URL del api después hay que **reconstruir la imagen** (re-ejecutar el workflow o hacer un nuevo push a `main`) y redesplegar el frontend.

## Documentación

- [`CLAUDE.md`](CLAUDE.md) — arquitectura, convenciones y comandos (la referencia más completa y actualizada).
- [`new-version.md`](new-version.md) — diseño original del v2 (intención, no siempre 100% al día con el código).
- [`docs/`](docs/) — guías de integración, plan de producción, seguimiento de trabajo en curso, `openapi.yaml`.
