# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repo is (v2)

QB Notify **v2** is a multi-tenant notification service written in **Go** (asynq + Redis Sentinel + PostgreSQL) with a **React admin SPA** (`frontend/`). Each backend that uses the service is a tenant **Application**; every tenant resource carries an `application_id`. The original design lives in `new-version.md` (still the reference for intent), but the code has since moved well beyond it — trust the code first, then `new-version.md`.

- **Backend:** Go, self-contained under `backend/` (its own `go.mod`/`go.sum`). Entry points in `backend/cmd/`, all logic in `backend/internal/`, SQL in `backend/db/`.
- **Frontend:** `frontend/` — Vite + React + TS + Tailwind v4 + shadcn/ui (see UI rules below).
- **Deploy:** `deploy/` — Dockerfiles for the 3 apps, the local infra `docker-compose.yml`, nginx template, `.env.*.example` per app, smoke/e2e scripts.
- **Docs:** `docs/` — progress/tracking for ongoing work, plus `openapi.yaml` (API contract). `new-version.md` (the design) stays at the repo root, untouched.

Top-level layout: `backend/` · `frontend/` · `deploy/` · `docs/` · `new-version.md` · `CLAUDE.md` · `README.md`. There is no legacy v1 left in the tree (the old Node/Prisma app was fully removed; no data migration existed from it).

### Flujo de branches

- **`feature/**`** — todo trabajo nuevo se hace en una branch `feature/<algo>`. Es la única que dispara `.github/workflows/ci.yml` (lint, test, build, smoke test) — el chequeo de desarrollo.
- **`staging`** — a donde se mergean las `feature/**` ya listas. Es lo que despliega Dokploy como entorno de **development**. No dispara CI ni los workflows de imagen; Dokploy construye directo del repo en este branch.
- **`main`** — **producción**. Solo recibe merges de `staging` una vez aprobado ahí (nunca directo desde `feature/**`). Cada push a `main` dispara `.github/workflows/{backend,services,frontend}.yml`, que compilan y publican las 3 imágenes a `docker.divergtech.com`.

En resumen: `feature/**` (CI) → `staging` (Dokploy dev) → aprobación → `main` (build + push de imágenes de producción).

## Commands

Everything goes through the **Makefile** at the repo root (Go lives in `~/sdk/go/bin` — add it to `PATH`). The Makefile `cd`s into `backend/` for every Go target, so run it from the repo root, not from inside `backend/`. It auto-loads `.env` (also at the repo root). Docker compose lives at `deploy/docker-compose.yml`; if you invoke compose directly, use the standalone `docker-compose -f deploy/docker-compose.yml` (the `docker compose` plugin isn't installed here).

```bash
make up             # infra: Postgres + Redis (master/replica/3 sentinels) + MailHog
make down / logs / ps

make run-api        # go run ./cmd/api      (HTTP API, default :8080)  — runs inside backend/
make run-worker     # go run ./cmd/worker   (asynq consumer + metrics :9091)
make build          # -> bin/api, bin/worker (at repo root)

make migrate-up     # apply migrations (goose)   · migrate-down / migrate-status
make seed           # base templates, super-admin, demo app (idempotent)
make reencrypt      # rotate secrets to the primary KMS key version

make test           # go test ./...   · vet / fmt / tidy / lint (golangci-lint)
make sqlc           # regenerate backend/internal/store/sqlc from backend/db/queries (needs Docker)
make smoke          # infra + API + /readyz check

# Frontend (in frontend/):
npm run dev         # Vite dev server on :5173, proxies /admin, /v1, /healthz -> :8080
npm run build       # tsc && vite build
npm run test        # vitest run   (test:watch for watch mode)
```

Typical dev loop: `make up` → `make migrate-up && make seed` → `make run-api` + `make run-worker` (separate terminals) → `npm run dev` in `frontend/`.

## Architecture

Two long-running Go processes share Postgres + Redis (all under `backend/`):

- **`cmd/api`** — chi HTTP server. Public tenant API under `/v1` (HMAC auth) and admin API under `/admin` (JWT). Health at `/healthz` (+ `/health` alias), `/readyz`; Prometheus at `/metrics`.
- **`cmd/worker`** — asynq consumer over 3 weighted queues (`critical`:6, `default`:3, `low`:1). Also runs the periodic-task manager (recurring notifications) and an hourly PII-retention sweep. Exposes its own metrics on `:9091`.

Other entry points: `cmd/migrate` (goose CLI: `up`/`down`/`status`), `cmd/seed` (idempotent seed), `cmd/reencrypt` (KMS key rotation).

**Request layering (API):** `internal/http/router.go` → middleware (auth + scopes + rate limit) → `internal/http/handlers/*` → domain services (`internal/notification`, `internal/template`, `internal/quota`, …) → `internal/store` (sqlc queries over a pgx pool).

**`internal/` packages:** `auth` (HMAC, JWT, Argon2id admin login, rate-limit, replay guard) · `channels` (Sender interface + Dispatcher; `email` via go-mail/SMTP, `sms` via Twilio, `push` FCM/APNS, `inapp` DB inbox) · `config` (viper, 12-factor) · `crypto` (AES-256-GCM versioned keyring + Argon2id) · `notification` (service = validate/persist/enqueue; processor = worker-side render/route/dispatch) · `queue` (asynq client/server, recurring, webhook tasks) · `quota` · `recurring` · `retention` · `store` (pgx pool + generated sqlc) · `template` (Handlebars render + variable/JSON-schema extraction) · `webhook` (async delivery, backoff, circuit breaker) · `observability` (slog, request IDs).

All paths above (`cmd/`, `internal/`, `db/`) are relative to `backend/`.

### Notification send flow

1. **Enqueue (API):** `notification.Service.Create` validates input, resolves the template, checks quota + opt-out, then in **one `db.Tx`** inserts the `notifications` row (`PENDING`) and enqueues a `notification:send` asynq task keyed by `notification_id`. **The transaction commits before the enqueue** — never reorder this (a rollback after enqueue would orphan the task).
2. **Process (worker):** `notification.Processor` loads the notification from the DB (the task payload is just the id — DB is authoritative), renders the template, resolves the `channel_route` to an SMTP connection or channel provider, dispatches via `channels.Dispatcher`, then records a `delivery_attempts` row and updates status.
3. **Retries:** asynq drives re-execution (exponential backoff, `QUEUE_RETRY_MAX`). `delivery_attempts` + `notification_logs` mirror that state in Postgres for auditing — keep both consistent when touching retry logic. `notification_logs` is append-only (`created`, `queued`, `sent`, `retry`, `failed`, …).

### Data layer (sqlc)

- Migrations are **goose** files in `backend/db/migrations/` (`-- +goose Up`/`Down` markers required). Latest: `00008_admin_token_version.sql` (`token_version` en `admin_users`, para revocación de sesiones admin).
- Queries are hand-written in `backend/db/queries/*.sql`; `make sqlc` generates Go into `backend/internal/store/sqlc` (`backend/sqlc.yaml`, pgx/v5, `emit_interface`). **After editing `db/queries/*.sql`, run `make sqlc`.** Don't hand-edit generated code.
- `store.DB.Tx(ctx, fn)` gives a transactional `*sqlc.Queries`. **Every tenant query must filter by `application_id`** — a missing filter is a cross-tenant data leak.

### Template modes: `builder` vs `html`

Cada plantilla (y `base_template`) lleva un **`kind`**:

- **`builder`** — el editor visual por bloques: `structure` (JSON) se compila a `body` (HTML) en `internal/template/builder.go`.
- **`html`** (solo `EMAIL`) — HTML/CSS crudo que el autor edita; se guarda **saneado** en `body`. `structure` conserva los últimos bloques para poder "volver a visual" en la SPA.

Clave: **el envío no distingue kind** — ambos producen `body` (HTML con `{{vars}}`) que el worker renderiza con raymond (`internal/template/render.go`). La rama por kind vive en `template.Service` (`compileBody`, `persistVersion`, `PreviewDraft`).

- **Saneador** (`internal/template/sanitize.go`, `x/net/html` con allowlist, se aplica al **guardar y al previsualizar**): elimina `script`/`iframe`/`svg`/`math`/`on*`/`javascript:`; conserva `<style>` y estilos inline; límite `MaxEmailHTMLBytes` (256 KB). El **triple-stache `{{{ }}}` se colapsa a `{{ }}`** para forzar el auto-escape de raymond (si no, `{{{payload}}}` inyectaría HTML sin sanear — era un bypass real ya cerrado).
- **Seed**: `cmd/seed` siembra 13 bases `builder` + 12 bases `html` (del export v1, claves `*-html`), con el logo de marca embebido como data URI.
- **SPA**: `EmailBuilder` (bloques, dnd-kit) vs `HtmlBuilder` (CodeMirror); ambos comparten `VariablesPanel` y previews con **`sandbox=""`** (sin scripts). La ruta del editor se carga en diferido (lazy).

## Auth

- **Public API (`/v1`):** HMAC-SHA256 per request. An Application has one or more `api_keys` (public `key_id` + encrypted `secret`). Client signs method/path/canonical-query/body-hash/timestamp; headers `X-QBN-Key-Id`, `X-QBN-Timestamp`, `X-QBN-Signature`. Timestamp skew `HMAC_TIMESTAMP_SKEW_SECONDS` (default 300), Redis replay guard, per-key rate limit. Keys carry **scopes** (`notifications:read`, `notifications:send`).
- **Admin SPA (`/admin`):** email + password (Argon2id, constant-time dummy-hash to prevent enumeration) → short-lived access token (kept client-side, `Authorization: Bearer`) + a refresh token in an **HttpOnly cookie** (rotated on use; see `CookieConfig`/`CORS_ALLOWED_ORIGINS` in config). Roles: `SUPER_ADMIN` (null `application_id`, full access) vs `APP_ADMIN` (bound to one app). `/admin/auth/login`, `/refresh` and `/logout` are public but IP-rate-limited (login/refresh).
- **Public routes (no auth):** `/healthz`, `/health`, `/readyz`, `/metrics`, and the admin login/refresh/logout endpoints.
- **CORS:** only needed for `/admin` when the SPA is served from a different origin than the API (no proxy in front) — see `CORS_ALLOWED_ORIGINS` below and `internal/http/cors_middleware.go`. `/v1` doesn't need it (server-to-server, HMAC-signed, not called from a browser).

## Crypto & secrets

`internal/crypto` uses AES-256-GCM with a **versioned keyring**: ciphertext is `[version][nonce][ct+tag]`, encrypted with the primary version, decrypted against any version — so key rotation is zero-downtime. Encrypted columns: `api_keys.secret_enc`, `smtp_connections.password_enc`, `channel_providers.config_enc`, `webhook_endpoints.secret_enc`. To rotate: add the new key + bump `SECRET_ENCRYPTION_KEY_VERSION`, restart api/worker, then `make reencrypt`. **If `SECRET_ENCRYPTION_KEY` is unset/invalid, secret-creating operations (seed, key/SMTP/provider creation) fail** — set it before seeding.

## Config & environment

Copy `.env.example` (repo root) → `.env`. viper maps env vars → nested config (`_`→`.`); a malformed value fails at startup, not first use. Key vars:

- **Server:** `APP_ENV`, `API_PORT` (default 8080), `WORKER_CONCURRENCY` (10), `METRICS_PORT` (9091).
- **Postgres:** `DATABASE_URL` (dev points at `localhost:55432`, not 5432 — see below).
- **Redis:** Sentinel mode via `REDIS_SENTINELS` + `REDIS_MASTER_NAME` (+ `REDIS_SENTINEL_PASSWORD`); or single-node `REDIS_ADDR`. Plus `REDIS_PASSWORD`, `REDIS_DB_{DEFAULT,LOCKS,ASYNQ}`, `REDIS_PREFIX`.
- **Crypto/JWT:** `SECRET_ENCRYPTION_KEY(_VERSION)`, `SECRET_ENCRYPTION_KEYS`, `ADMIN_JWT_SECRET`.
- **CORS (admin API):** `CORS_ALLOWED_ORIGINS` — CSV of exact origins allowed to call `/admin` from the browser. Only needed when `frontend/` is deployed without a reverse proxy in front of the api (see `deploy/env.backend.example`).
- **SSRF guards (default strict):** `ATTACHMENT_ALLOW_PRIVATE`, `ATTACHMENT_HOST_ALLOWLIST`, `WEBHOOK_ALLOW_PRIVATE`, `TRUST_PROXY_HEADERS`.
- **Seeding:** `SEED_ADMIN_EMAIL`/`_PASSWORD`, `SEED_API_KEY_SECRET`, `SEED_SMTP_HOST` (MailHog in dev).

`deploy/docker-compose.yml` (host ports, local infra only): **Postgres 55432**, Redis master 6379 / replica 6380, sentinels 26379/26380/26381, MailHog SMTP 1025 / UI 8025. Production deploy (Dokploy) is documented in `deploy/deploy-staging.md`; each app (`api`/`worker`/`web`) has its own `deploy/env.*.example` and Dockerfile.

## Frontend / SPA (`frontend/`) — UI rules

The admin SPA (Vite + React + TS + **Tailwind v4**) **must use shadcn/ui + Tailwind**. Hard rule for any UI work:

- **Use shadcn/ui components** from `frontend/src/components/ui/` (built on Radix). Don't hand-roll UI primitives — if one is missing, add it in the shadcn style (CVA variants + the `cn()` helper from `@/lib/utils`).
- **Style with Tailwind + shadcn design tokens** (`bg-primary`, `text-muted-foreground`, `border-border`, `bg-card`, `ring-ring`, `text-destructive`, …) from `frontend/src/index.css`. Brand `--primary` is emerald. Avoid hard-coded `gray-*`/`emerald-*` in new UI.
- **Icons:** `lucide-react`. **Imports:** the `@/` alias (→ `frontend/src/`).
- The SPA talks to the API through `src/api/client.ts` (access token in memory/localStorage as `Authorization: Bearer`, refresh via HttpOnly cookie, Problem+JSON errors). In dev, `vite.config.ts` proxies `/admin`, `/v1`, `/healthz` to `:8080`. In prod there's no proxy — the bundle calls the api's public URL directly via `VITE_API_URL` (a **build-time** arg, not a runtime env var; see `deploy/env.frontend.example`).

## Conventions & gotchas

- **Redis Sentinel needs `network_mode: host`** (in the compose file) so sentinels advertise reachable addresses — works on **Linux only**. On macOS/Windows, comment out `REDIS_SENTINELS` and set `REDIS_ADDR=localhost:6379`.
- **`backend/internal/store/sqlc` is generated** — never hand-edit; run `make sqlc` (Docker) after touching `db/queries/*.sql` or the schema.
- **`admin_users` CHECK constraint:** `SUPER_ADMIN` ⇒ null `application_id`, `APP_ADMIN` ⇒ non-null. Violating it fails the insert.
- **Recipient JSON** on a notification may hold `email` / `phone` / `push_token` / `name`; the channel decides which is required. Missing the required field ⇒ the worker-side send fails (the API validates minimally).
- **Comments and docs in this repo are in Spanish** — match the surrounding language when editing.
- The `go` toolchain is under `~/sdk/go/bin`; export it onto `PATH` before running Go commands directly. Run Go commands from `backend/` (or via the root Makefile, which `cd`s there for you) — `backend/go.mod` is the module root.
- **Docker build context for `deploy/Dockerfile.{backend,services,frontend}` is the repo root**, not `deploy/` and not `backend/`/`frontend/` alone — the Dockerfiles `COPY backend/...` / `COPY frontend/...` / `COPY deploy/default.conf.template` from there. In Dokploy: root/context = `.`, Dockerfile path = `deploy/Dockerfile.<app>`.
- **Production images** (`notify-backend`, `notify-services`, `notify-frontend`) are built and pushed to `docker.divergtech.com` by `.github/workflows/{backend,services,frontend}.yml` on every push to `main`. Naming mirrors other divergtech-dev projects (e.g. `call-center`): `backend`/`services`/`frontend`, not `api`/`worker`/`web`.
