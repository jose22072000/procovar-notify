# Plan — poner v2 en producción

> Lo hacemos por pasos, uno a uno. Marca con ✅ al cerrar cada uno.
> **Quién:** 👤 = tú (decisión/ops/credenciales) · 🤖 = yo (código/config en el repo).

## Paso 1 — PR y merge a `main` 👤
- Abrir el PR: `feature/cambios-ui-v2` → `main`
  (`https://github.com/divergtech-dev/qb-notify/compare/main...feature/cambios-ui-v2?expand=1`).
- Esperar el **CI en verde** (backend + frontend + smoke).
- Revisar y **mergear** (al ser ~117 commits, valorar *squash* o *merge* normal).
- Tras el merge, `main` pasa a ser el v2.

## Paso 2 — Dockerfiles del v2 ✅ 🤖
- `deploy/Dockerfile.backend` — Go, multi-stage → binario estático en distroless. **26 MB**.
- `deploy/Dockerfile.services` — igual, `./cmd/worker`. **27 MB**.
- `deploy/Dockerfile.frontend` — Node build + nginx; `deploy/default.conf.template` (solo
  estáticos + fallback SPA; el backend es público, sin proxy — el bundle usa `VITE_API_URL`
  horneado en build-time). **64 MB**.
- `.dockerignore` en la raíz (contexto mínimo, excluye node_modules/.git/v1).
- **Verificado en local:** las 3 imágenes construyen; el backend arranca (valida config) y
  el frontend sirve la SPA (`/` y rutas → 200). `GOPROXY` es build-arg (mirror opcional).
- ✅ `.github/workflows/{backend,services,frontend}.yml` publican `notify-{backend,services,frontend}`
  a `docker.divergtech.com` en cada push a `main` (mismo patrón que `call-center`).

## Paso 3 — Configuración y secretos de producción 👤🤖
- 🤖 ✅ `.env.*.example` por app (para Dokploy): `deploy/env.backend.example`,
  `env.services.example`, `env.frontend.example`, `env.seed.example`. Cada uno lista sus
  variables y el **puerto a exponer** (backend 8080 · services 9091 · frontend 80).
- 👤 Generar secretos reales (nunca los de dev): `SECRET_ENCRYPTION_KEY`,
  `ADMIN_JWT_SECRET`, credenciales de BD/SMTP, `SEED_ADMIN_PASSWORD`, `SEED_API_KEY_SECRET`.
- Decidir gestión de secretos (vault del proveedor, secrets de K8s, `.env` protegido…).

## Paso 4 — Infraestructura 👤
- **Postgres** gestionado (o contenedor con backups).
- **Redis** con Sentinel (o el equivalente HA del proveedor); si es single-node, usar
  `REDIS_ADDR` en vez de `REDIS_SENTINELS`.
- **SMTP real** (SendGrid/SES/Postmark/servidor propio) — MailHog es solo para dev.
- (Opcional) proveedores Push/SMS reales (FCM/APNS/Twilio) si se usan esos canales.

## Paso 5 — Migraciones y seed en el entorno ✅ 🤖 (automático en el api)
- El **api aplica las migraciones al arrancar** (`AUTO_MIGRATE`, por defecto on) y, si
  la BD está **limpia** (sin admins), **siembra** el super-admin (`SEED_ADMIN_*`) + las
  25 plantillas base (`AUTO_SEED`, por defecto on; sin app demo en prod).
- Migrate/seed son reutilizables (`db.Migrate`, `internal/seed`) y siguen disponibles a
  mano (`cmd/migrate`, `cmd/seed`) para re-siembras puntuales — ver `env.seed.example`.
- **Verificado** en local contra una BD limpia: 8 migraciones, super-admin + 25 bases,
  0 apps demo en modo producción.

## Paso 6 — Despliegue de api + worker 👤🤖
- Orquestación: Compose/K8s/systemd. **Dos procesos**: `api` (HTTP) y `worker` (asynq).
- Healthchecks (`/readyz`), reinicio automático, réplicas del worker si hace falta.
- **Métricas** Prometheus del worker (`:9091`) y logs centralizados.

## Paso 7 — Exposición y hardening final 👤🤖
- **TLS + dominio** y reverse proxy (nginx/traefik/ingress) delante del API.
- Si va tras proxy, `TRUST_PROXY_HEADERS=true` (solo si el proxy es de confianza).
- Repasar guardas: `ATTACHMENT_ALLOW_PRIVATE`, `WEBHOOK_ALLOW_PRIVATE`, rate limits,
  CORS del panel, y rotación de claves (`make reencrypt`).
- Alta de la primera **Application** (tenant) real + su API key desde el panel.

## Estado
- Paso 1: ⏳ en curso (lo haces tú). Resto: pendientes, uno a uno.
