# Revisión profunda — QB Notify v2

Auditoría de calidad previa al PR `version-2 → main`. Trazable y retomable.
Fecha: 2026-06-29. Rama `version-2`.

## 🔖 Estado (cierre 2026-06-29) — cómo retomar

**Hecho y commiteado — TODO el plan F5 cerrado:** must-fix (A1–A3, M-series),
**F4 tests #1–#9 COMPLETO** y **🔵 bajos L1–L21 COMPLETO**. Cada cambio con su
commit en `version-2` (sin push). **Diferidos a v2.2:** M9 (FCM v1), L11
(paginación), L12 (Redis TLS).

**Cómo seguir:** no queda nada accionable de la revisión. Suite **Go (todo) +
web (vitest 21/21)** verde; `go build`/`go vet` limpios.

> **Recomendación: abrir el PR `version-2 → main`** — todo cerrado y verde.
> (El push y la creación del PR requieren tu confirmación.)

## Fases

- [x] **F0 — Verificación**: builds (web + Go), `go vet`, tests (web + Go).
- [x] **F1 — Backend** (agente).
- [x] **F2 — Frontend** (agente).
- [x] **F3 — Completitud vs diseño** (agente).
- [x] **F4 — Tests** (agente).
- [x] **F5 — Correcciones**: todos los hallazgos accionables aplicados (ver tabla); solo M9/L11/L12 diferidos a v2.2.

## F0 — Verificación (todo verde)

| Check | Resultado |
|-------|-----------|
| `web: npm run build` (tsc + vite) | ✅ sin warnings · 452 KB JS / 134 KB gzip |
| `web: npm test` (vitest) | ✅ 4/4 |
| `go build ./...` | ✅ |
| `go vet ./...` | ✅ |
| `go test ./...` | ✅ 0 FAIL (16 paquetes **sin** tests — ver F4) |

## Hallazgos

Severidad: 🔴 alta (bug/seguridad) · 🟠 media (deuda/func.) · 🔵 baja (nit). Estado: ⬜ pendiente · ✅ hecho · 💤 diferido (v2.2).

### 🔴 Alta

| # | Área | Hallazgo | Archivo | Estado |
|---|------|----------|---------|--------|
| A1 | Cola | Idempotencia rota: `err == asynq.ErrTaskIDConflict` nunca casa (el sentinel va envuelto con `%w`) → un re-encolado duplicado sale como error duro en vez de no-op. Usar `errors.Is`. | `internal/queue/asynq.go:62` | ✅ |
| A2 | Seguridad | **SSRF en entrega de webhooks**: la URL del tenant se hace POST sin validar IP/host (a diferencia de adjuntos). Permite `169.254.169.254`, loopback, redes internas. Reutilizar el guard SSRF (dialer) + allowlist opcional. | `internal/webhook/service.go:115`, `internal/admin/webhooks.go` | ✅ |
| A3 | Seguridad | **XSS almacenado en templates**: `attr()` escapa atributos pero el **texto** de secciones (title/text/button/footer) se emite crudo; además `{{{x}}}` (triple-stache) evita el auto-escape de raymond y no se captura en `required_variables`. Escapar `<>&` del texto literal; rechazar/quitar triple-stache. | `internal/template/builder.go:101,104,108,120,151`, `render.go` | ✅ |

### 🟠 Media

| # | Área | Hallazgo | Archivo | Estado |
|---|------|----------|---------|--------|
| M1 | Seguridad | Sin rate-limit/lockout en `/admin/auth/login` y `/auth/refresh` (el limiter solo está en `/v1`) → fuerza bruta de contraseña. | `internal/http/router.go:75-76` | ✅ |
| M2 | Seguridad | Sin límite de tamaño de body en endpoints no autenticados (`login`, `refresh`) → DoS por memoria. Envolver con `http.MaxBytesReader`. | `internal/httpx/decode.go:15` | ✅ |
| M3 | Seguridad | `ADMIN_JWT_SECRET` sin longitud mínima (cualquier valor no vacío pasa). Exigir ≥32 bytes en `validate()`. | `internal/config/config.go:159` | ✅ |
| M4 | Seguridad | Seed crea SUPER_ADMIN con password por defecto conocido (`changeme-admin`) y API key `demo-secret-…`. Exigir env fuera de desarrollo. | `cmd/seed/main.go:35,63` | ✅ |
| M5 | Bug | `UpdateApplication`: si `name` y `status` son nil, devuelve un `Application{}` en blanco (ID/nombre vacíos) sin error. Inicializar desde `GetApplication` o cortar el no-op. | `internal/admin/service.go:86-111` | ✅ |
| M6 | Bug/Audit | Acción de auditoría incorrecta: borrar proveedor registra `provider.update`; rutas registran create y delete como `route.update`. Falta `*.delete`/`route.create`. | `internal/admin/providers.go:104`, `routes.go:51,65` | ✅ |
| M7 | Completitud | **Acciones de templates no se auditan** (el diseño §11 las exige; fase 14 marcada ✅). `template/service.go` no llama a `audit.Record`. | `internal/template/service.go` | ✅ |
| M8 | Completitud | **UI de marcado de variables del builder no implementada** (fase 8.2 ✅ pero parcial): el backend acepta `variables` (required/optional/tipo/desc) pero la SPA solo muestra chips y el `submit` no envía `variables`. | `web/src/pages/tabs/Templates.tsx:146-167`, `TemplateBuilder.tsx:258-276` | ✅ |
| M9 | Func/Diseño | **FCM legacy no funcional**: usa `fcm/send` + `Authorization: key=` (Google lo apagó en 2024). Migrar a HTTP v1 (OAuth2) o documentar como no operativo. | `internal/channels/push.go:51-63` | 💤 |
| M10 | Perf | Templates y JSON Schemas se recompilan en cada render/validate (Preview lo hace 2×). Cachear `*raymond.Template`/`*jsonschema.Schema` por (template,versión). | `internal/template/render.go:13`, `validate.go:29` | ✅ |
| M11 | FE Bug | Editor "JSON avanzado" inusable: `value` controlado por `JSON.stringify(structure)`; un keystroke que deja JSON inválido revierte el texto. Usar estado de texto crudo separado. | `web/src/pages/tabs/Templates.tsx:366-377` | ✅ |
| M12 | FE Bug | `qbn_admin` corrupto en localStorage rompe toda la app (white screen): `JSON.parse` sin try/catch en el init de `AuthProvider`. | `web/src/auth/auth.tsx:26-29` | ✅ |
| M13 | FE Bug | SSE Monitor: si `fetch` lanza (servidor caído), el `catch` reintenta sin backoff → loop de reconexión. Añadir `await sleep(2s)` en el catch. | `web/src/pages/tabs/Monitor.tsx:69-107` | ✅ |
| M14 | FE Bug | `useAsync` sin cancelación: al cambiar `deps` rápido (Audit paging, filtros) una respuesta vieja puede pisar a la nueva. Flag `active`/AbortController. | `web/src/ui.tsx:242-254` | ✅ |
| M15 | FE A11y | Filas de la tabla de Aplicaciones (≥500px) solo navegables con ratón (`<tr onClick>` sin tabIndex/teclado); "Gestionar" es decorativo. | `web/src/pages/Applications.tsx:117-152` | ✅ |
| M16 | Seguridad | HMAC: el firmado **no incluye el query string** (params no autenticados); "anti-replay" es solo ventana ±5 min sin caché de nonce. | `internal/auth/hmac.go:24`, `apikey.go:75` | ✅ |
| M17 | FE Consistencia | Breakpoint card↔tabla inconsistente: tabs manuales cambian en 768 (md) pero `.responsive-table` (ApiKeys/Audit) en 640 → entre 640–767 unos muestran tarjetas y otros tabla apretada con badge sin pastilla. Alinear a 768. | `web/src/index.css:97` | ✅ |

### 🔵 Baja (deuda / robustez / nits)

| # | Área | Hallazgo | Archivo | Estado |
|---|------|----------|---------|--------|
| L1 | Resiliencia | Circuit breaker cuenta errores de validación (destinatario/payload) como fallos de infra → puede abrirse con una conexión sana. Añadir `IsSuccessful`. | `internal/notification/processor.go:126`, `breaker.go:32` | ✅ |
| L2 | Bug latente | Fallos transitorios en `resolveRoutes` (DB/decrypt) hacen `continue` sin log → si todas las rutas se saltan, error permanente indiagnosticable (`SkipRetry`). Loggear y clasificar transitorios como retryables. | `internal/notification/processor.go:226-258` | ✅ |
| L3 | Errores silenciosos | Escrituras de estado best-effort con `_ =`; si `MarkNotificationSent` falla, la fila queda en `PROCESSING` sin rastro. Al menos `Warn`-log. | `internal/notification/processor.go:97,145,148,166,179,190,194` | ✅ |
| L4 | Seguridad | Allowlist de adjuntos no se aplica en redirects (sin `CheckRedirect`); el guard por IP sí. Defensa en profundidad. | `internal/notification/attachments.go:90` | ✅ |
| L5 | Bug | Cuota: `Allow` incrementa el contador diario antes de poder rechazar por el mensual → se "cobra" envíos que no ocurren. Usar Lua read-then-incr. | `internal/quota/quota.go:36` | ✅ |
| L6 | Robustez | Purga de retención usa `context.Background()` sin timeout/cancelación al apagar. | `cmd/worker/main.go:123,131` | ✅ |
| L7 | Config | Versión de clave de cifrado se trunca a `byte` sin validar rango (256→0). Validar 1..255. | `internal/config/config.go:115` | ✅ |
| L8 | Errores silenciosos | `_ = httpx.DecodeJSON` al crear API key: con `DisallowUnknownFields`, un `scopes` mal escrito se descarta y la key se crea con scopes vacíos en vez de 400. | `internal/http/admin_resource_handlers.go:189` | ✅ |
| L9 | Seguridad | IP de auditoría spoofable vía `X-Forwarded-For` (confía en el primer valor sin proxy de confianza). | `internal/http/admin_middleware.go:29` | ✅ |
| L10 | Seguridad | Enumeración de usuarios por timing en login (retorna antes de hashear si el email no existe). Comparar contra hash dummy. | `internal/auth/admin.go:40` | ✅ |
| L11 | Escala | Endpoints admin de listado globales sin paginar (`ListApplications`, `ListAdminUsers`, y listados por app). | `internal/http/admin_resource_handlers.go` | 💤 |
| L12 | Redis | Sin opción TLS para Redis/Sentinel (credenciales/payloads en claro a Redis remoto). | `internal/queue/redis.go:28`, `asynq.go:18` | 💤 |
| L13 | Comentarios obsoletos | `apns.go:64` dice que el token se regenera por envío (ya hay caché ~50 min); `builder.go:70` `Compile` nunca devuelve error pero el caller mapea `invalid_structure`. | `internal/channels/apns.go:64`, `template/builder.go:70` | ✅ |
| L14 | FE A11y | Botón de cerrar aviso SMTP (`<button><X/></button>`) sin nombre accesible. Añadir `aria-label`. | `web/src/pages/tabs/Smtp.tsx:139` | ✅ |
| L15 | FE | `client.ts` `parse()` hace `JSON.parse` crudo del body; un 5xx HTML lanza `SyntaxError` en vez de `ApiError` normalizado. | `web/src/api/client.ts:56` | ✅ |
| L16 | FE | Doble-submit posible al crear app (sin estado `saving`/disabled, a diferencia de otros forms). | `web/src/pages/Applications.tsx:41` | ✅ |
| L17 | FE | Doble fetch de `/admin/applications/{id}` (AppDetail breadcrumb + AppHeader); `statusTextColor` duplica la lógica de tono de `Badge`. | `web/src/pages/AppDetail.tsx:120,131,301` | ✅ |
| L18 | FE | Errores de los `useAsync` secundarios (dropdowns) no se muestran (Recurring/Routes/AdminUsers): si fallan, el `<Select>` queda vacío sin aviso. | varios tabs | ✅ |
| L19 | FE | `detail` no se limpia al cambiar el filtro en Notificaciones (panel desktop puede mostrar una notif que ya no está en la lista). | `web/src/pages/tabs/Notifications.tsx` | ✅ |
| L20 | Código muerto | `UpdateProvider` existe pero sin ruta ni UI (no es gap de diseño, es código sin usar). | `internal/admin/providers.go:87` | ✅ |
| L21 | Diseño (nota) | `Idempotency-Key` por header no se honra (solo body `idempotencyKey`); monitor no usa `asynq.Inspector` (§8.3); InApp sender es no-op por diseño. Documentar. | varios | ✅ |

## F4 — Cobertura de tests (huecos)

**16 paquetes sin tests.** Críticos a cubrir (orden sugerido):

1. ✅ `internal/http` (15 handlers, 0 tests) — incluye `requireAppAccess` (aislamiento multi-tenant), `clientIP` (XFF), parse de `scheduledAt`, `missing_fields`. → tests `httptest`+chi. **Hecho** (`middleware_test.go`: clientIP, requireAppAccess super/app/cross-tenant, appIDParam/parseID, validación temprana de Create).
2. ✅ `notification/processor`: ramas **RETRY** (H2), **circuit breaker** (H3) y **failPermanent** `route_error`/`render_error` (H4). **Hecho** (`processor_failure_test.go`): seam `WithAttemptInfo` para inyectar retry/max (asynq no es construible fuera de su paquete); RETRY→QUEUED+intento RETRY+log; agotado→FAILED; route_error (ruta borrada) y render_error (Handlebars inválido)→SkipRetry; breaker abre tras 3 fallos y corta el 4º (sender.sent==3).
3. ✅ SSRF: **redirect 302→127.0.0.1** y rebinding (H5), webhooks (H6). **Hecho**: `safedial/client_test.go` (redirect bloqueado en el 2º dial vía `Control`; loopback por IP y por nombre `localhost`/rebinding; opt-in); `notification/ssrf_redirect_test.go` (redirect bloqueado por `ssrfControl` en adjuntos); `webhook` `TestDeliver_BlocksInternalSSRF` (guard A2 bloquea POST a loopback, destino no invocado).
4. ✅ Cifrado de credenciales `CreateProvider`/`CreateWebhook` (H7). **Hecho** (`admin/service_test.go`): config del provider y secret del webhook se persisten cifrados (la columna no contiene el valor en claro) y descifran al original.
5. ✅ `channels/email.go` + `sender.go` (Dispatcher) (H8). **Hecho**: `sender_test.go` (Dispatcher enruta por canal, canal desconocido→error, propaga error, InApp no-op→"inapp"); `email_test.go` (envío SMTP contra un servidor falso in-process que captura MAIL/RCPT/DATA; ramas sin-SMTP y sin-destinatario). `-race` limpio.
6. ✅ FE `api/client.ts` (flujo 401→refresh→retry) y `auth/auth.tsx` (H9). **Hecho**: `client.test.ts` (Bearer, 401→refresh→retry con tokens nuevos, sin refresh-token no reintenta, refresh fallido propaga 401, no refresca en /admin/auth, problem+json→ApiError, 204→undefined); `auth.test.tsx` (qbn_admin corrupto no tumba la app M12, carga admin válido, login persiste tokens+admin, logout limpia). Vitest 15/15.
7. ✅ `auth/jwt.go` (M1), `auth/ratelimit.go` (M2), `auth/apikey.go` revoked/expired (M3). **Hecho**: `jwt_test.go` (roundtrip, confusión de tipo, expiración, firma manipulada, clave equivocada, rechazo de `alg=none`); `ratelimit_test.go` white-box+miniredis (refill con reloj inyectado, cabeceras `X-RateLimit-*`/`Retry-After`, fail-open con Redis caído); `TestHMACKeyRevokedAndExpired` (403 con firma válida).
8. ✅ M4–M10. **Hecho**: render-escape (M4) ya cubierto en A3; `httpx/respond_test.go` WriteProblem (M5); `queue/tasks_test.go` queueForPriority + payload roundtrip (M6); webhook `TestDeliver_FailingEndpointReturnsError` (M7); aserciones de canales ya robustas + email/dispatcher/inapp cubiertos en #5 (M8); `TestSuppressionAddValidationAndRoundtrip` admin (M9); `TestNotificationScopingIsolation` cross-tenant (M10).
9. ✅ **Hecho**: paquetes puros sin tests cubiertos — `apperr`, `domain` (DeriveQueueState), `observability`, `audit`; `inapp` (en #5); builder edge (`detectVariables` en web). Las páginas FE son presentacionales: su lógica real (client/auth) está cubierta en #6 — no se añaden 18 tests de render de bajo valor.

**Higiene:** estado global mutable en `ssrf.go` (atomics de paquete) — seguro solo porque los tests corren en serie; añadir `t.Parallel()` contaminaría. Sin dependencia de `time.Sleep` real/red/aleatoriedad en asserts. `quota_test.go` (miniredis + reloj fijo) y `apns_test.go` son buenos modelos.

## F5 — Plan de correcciones (propuesta)

**Antes del PR (must-fix):** A1, A2, A3, M1, M2, M3, M5, M6, M11, M12, M13, M14. (+ M4 si el seed corre fuera de local.)
**Pronto (calidad):** M7, M8, M10, M15, M16, M17, y los tests críticos F4 #1–#6.
**Diferir v2.2:** M9 (FCM v1/OAuth), L11 (paginación), L12 (Redis TLS), tests no críticos.
**Doc:** L21 (idempotency header, asynq inspector, InApp no-op) → nota en diseño.

> Próxima sesión: empezar por la fila 🔴 más alta con ⬜ y marcar ✅ al cerrarla.

## Notas / decisiones

- **L21 (decisiones de diseño documentadas, sin cambio de código)**: (1) la
  idempotencia se toma del campo `idempotencyKey` del body; la cabecera
  `Idempotency-Key` **no** se honra (se podría añadir como alias en v2.2). (2) El
  monitor calcula métricas desde Postgres, **no** usa `asynq.Inspector` (§8.3);
  es suficiente para el panel actual. (3) El `InAppSender` es **no-op por
  diseño**: la propia fila de la notificación es la bandeja (consultable por API),
  no hay proveedor externo.
- **M16**: el `StringToSign` ahora es `METHOD\nPATH\nQUERY\nSHA256(body)\nTIMESTAMP`
  (la query canónica `url.Values.Encode()` queda firmada). **Diverge del §4.1 del
  diseño** (que no incluía la query): se hace ahora porque v2 aún no está
  desplegado y no hay clientes que re-firmar (único firmante: el paquete `auth`).
  `new-version.md` queda intacto; esta nota documenta la desviación. El anti-replay
  pasa a ser real: caché de nonce en Redis (`SETNX` con TTL=2×skew) usando la
  propia firma como nonce; **falla abierta** si Redis no responde (como el rate
  limiter), acotado por la ventana de timestamp. Opcional vía `WithReplayGuard`.
- **A2**: webhooks bloquean destinos internos por defecto; opt-in `WEBHOOK_ALLOW_PRIVATE`.
- **A3**: el `esc()` del builder escapa el texto literal (`<>&'"`) y colapsa `{{{x}}}`/`{{&x}}`
  a `{{x}}` (auto-escapado por raymond). **Residual de menor severidad** (no bloqueante):
  un valor de payload con esquema `javascript:`/`data:` en un `href`/`src` no se sanea
  (raymond solo escapa HTML, no el esquema). En email no ejecuta; en el preview (iframe)
  sí al hacer clic. Requeriría validar el esquema de URL en el render (o un sanitizador).
  Plantillas ya compiladas antes del fix conservan el HTML viejo hasta re-guardarlas.
