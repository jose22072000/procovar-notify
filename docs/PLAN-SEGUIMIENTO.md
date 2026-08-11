# Plan de seguimiento — QB Notify v2

> Estado de implementación y plan de continuación. **`new-version.md` es el
> documento de diseño y no se modifica**; todo el seguimiento vive aquí.
> Rama: `version-2`. No se hace push hasta que el usuario lo indique.

## Alcance v2 — completado

| Fase | Contenido | Estado |
|------|-----------|--------|
| 0 | Cimientos del proyecto Go + infra local (compose, Sentinel, MailHog) | ✅ Hecho |
| 1 | BD (goose) + acceso a datos (sqlc/pgx) + cifrado AES-GCM + seeds | ✅ Hecho |
| 2 | Auth HMAC, JWT admin, scopes, rate limiting, multi-tenancy | ✅ Hecho |
| 3 | Esqueleto de envío (asynq + worker + Email/In-App) | ✅ Hecho |
| 4 | API pública `/v1` completa (notificaciones, inbox, preferencias, batch) | ✅ Hecho |
| 5 | Templates + template builder (backend) + JSON Schema | ✅ Hecho |
| 6 | API admin de recursos + auditoría (registro) + métricas + monitor de cola | ✅ Hecho |
| 7 | Resiliencia (fallback de rutas + circuit breaker), recurrentes, observabilidad | ✅ Hecho |
| 8 | SPA de administración (todas las pestañas) | ✅ Hecho |
| 8.2 | Template builder visual por secciones (drag&drop, variables) | ✅ Hecho |
| 10 | Envío real Push **FCM** y SMS **Twilio** | ✅ Hecho |
| 11 | Envío real Push **APNS** (token JWT ES256) | ✅ Hecho |
| 12 | Fallback de idioma (i18n) en el render | ✅ Hecho |
| 13 | Adjuntos en email (`attachments`, inline base64 + URL) | ✅ Hecho |
| 14 | Auditoría: endpoint admin (`AdminAuditLog`) + pestaña SPA + IP/UA | ✅ Hecho |

## Plan de continuación

Cada fase: tests propios y, cuando aplique, endpoint admin + pestaña SPA. Commit
independiente. Se marca aquí al cerrarla.

### Paso 2 — Endurecimiento (calidad, sin features nuevas)

| Fase | Contenido | Estado |
|------|-----------|--------|
| 15 | Anti-SSRF en adjuntos por URL: resolver IP destino y bloquear loopback/privadas/link-local; allowlist opcional de hosts | ✅ Hecho |
| 16 | Caché del token APNS (~1 h) por (teamId,keyId), thread-safe; regenerar al expirar | ✅ Hecho |
| 17 | Cobertura E2E adicional: firma HMAC, batch parcial, notificación programada (`scheduledAt`) | ✅ Hecho |

### Paso 3 — Roadmap v2.1 (§16 del diseño)

| Fase | Contenido | Estado |
|------|-----------|--------|
| 18 | **Webhooks de estado al tenant**: entidad `WebhookEndpoint` por app (url, secret, eventos); entrega asíncrona (asynq) con firma HMAC saliente y reintentos; disparada en SENT/DELIVERED/FAILED. Admin CRUD + pestaña SPA | ✅ Hecho |
| 19 | **Ingesta de eventos de proveedor (bounces/opens) + suppression list**: endpoint de ingesta, actualización de estado de la notificación, `suppression_list` por app; el envío salta destinatarios suprimidos. Admin/SPA | ✅ Hecho |
| 20 | **Cuotas por tenant**: límites de envío por app (diario/mensual), contadores en Redis, rechazo `429` al exceder, configurable por admin + SPA | ✅ Hecho |
| 21 | **Rotación de claves KMS**: keyring de `SECRET_ENCRYPTION_KEY` con varias versiones, re-cifrado de secretos al rotar sin downtime | ✅ Hecho |

### Cierre

| Fase | Contenido | Estado |
|------|-----------|--------|
| 22 | **Revisar la SPA con el usuario**: tras cerrar las mejoras/refactors (con tests), avisar al usuario para que levante la SPA, revise diseño/UX y comunique los ajustes (diseño personalizado); se iteran las correcciones | ⬜ Pendiente |

### Paso 4 — Edición avanzada de plantillas (modo HTML)

| Fase | Contenido | Estado |
|------|-----------|--------|
| 23 | **Doble enfoque builder + HTML avanzado** y las 12 plantillas en ambas bibliotecas. Plan detallado en [`PLAN-EDICION-AVANZADA.md`](PLAN-EDICION-AVANZADA.md) (datos+backend, seed 12 HTML, SPA con editor de código, hardening) | ✅ Hecho |

## Cómo levantar la SPA (para la fase 22)

- **Solo visual** (maquetar, sin datos): `cd web && npm install && npm run dev` → http://localhost:5173
- **Completa** (con datos): `make up && sleep 5 && set -a && . ./.env && set +a && go run ./cmd/migrate up && go run ./cmd/seed`; luego `go run ./cmd/api` y `go run ./cmd/worker` en terminales aparte; y `cd web && npm run dev`.
  - Login: `admin@qbnotify.local` / `changeme-admin` · MailHog: http://localhost:8025 · Apagar: `make down`
- Estilos en **Tailwind**; componentes base (botones, inputs, tablas, badges) en `web/src/ui.tsx` — punto central para el diseño personalizado.

## Notas/decisiones

- Roadmap v2.1 = §16 del diseño ("No incluidos (roadmap)").
- Adjuntos (fase 15): guard anti-SSRF por IP resuelta (loopback/privadas/link-local) en `internal/notification/ssrf.go`, con opt-in `ATTACHMENT_ALLOW_PRIVATE` y **allowlist opcional de hosts** `ATTACHMENT_HOST_ALLOWLIST` (defensa en profundidad). Wireado en `cmd/worker`.
- APNS (fase 16): el token JWT se cachea ~50 min por clave (thread-safe) en `internal/channels/apns.go`.
