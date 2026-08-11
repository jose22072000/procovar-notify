# Bloque B — endurecimiento de sesión del panel

Auditoría de la SPA (`web/`) del 2026-07-09. El **Bloque A** (bugs funcionales +
robustez) y **B1** (endurecimiento de la sesión admin) ya están **aplicados**.
Queda pendiente **B2** (atomicidad del guardado de app), documentado abajo.

## B1 · Refresh token en cookie HttpOnly + revocación — ✅ APLICADO

### Qué cambió
- El **refresh token** ya no vive en `localStorage` (accesible a JS): se emite en
  una **cookie `HttpOnly`** (`qbn_refresh`, `Path=/admin/auth`), así un XSS no
  puede robarlo. El **access token** (corto, 15 min) sigue en `localStorage`; si
  lo roban, caduca en minutos y no es renovable sin la cookie.
- **Revocación**: nueva columna `admin_users.token_version` (migración
  `00008`). El refresh JWT lleva la versión con la que se emitió; el endpoint de
  refresh la compara con la BD. **Logout** (`POST /admin/auth/logout`) la
  incrementa → todos los refresh anteriores dejan de valer al instante.
- **Endpoints**: `login` y `refresh` fijan/rotan la cookie; `refresh` lee la
  cookie (ya no el cuerpo); nuevo `logout` (público e idempotente: funciona con
  la sesión ya caducada). CORS emite `Allow-Credentials: true` reflejando solo
  el origen de la allowlist.
- Cobertura: `internal/http/auth_handlers_test.go` (flujo cookie + revocación
  end-to-end) y `internal/auth/jwt_test.go` (versión en el token).

### Configuración por entorno (ver `.env.example`)
La cookie es configurable porque sus atributos dependen de la topología:

| Escenario | `COOKIE_SAMESITE` | `COOKIE_SECURE` | Extra |
|---|---|---|---|
| Panel y API **mismo dominio** / un reverse proxy | `lax` | `true` (prod HTTPS) | `COOKIE_DOMAIN` vacío |
| Panel y API **dominios distintos** | `none` | `true` (obligatorio) | `CORS_ALLOWED_ORIGINS` = origen del panel |
| Dev (proxy de Vite, http) | `lax` | `false` | defaults |

`COOKIE_SAMESITE=none` con `COOKIE_SECURE=false` **falla al arrancar** (los
navegadores descartan esa cookie). CSRF queda cubierto por `SameSite`: los
endpoints de mutación de `/admin/*` usan `Authorization: Bearer` (no cookie), y
la cookie solo la usan `refresh`/`logout`, que `SameSite=Lax` protege.

### Nota de despliegue en producción (una sola vez)
Al desplegar, la sesión admin **actual** (esquema viejo, refresh en
`localStorage`, sin cookie) se cerrará **una vez**: cuando el access caduque, el
refresh por cookie fallará → te manda al login. **Vuelves a entrar y el nuevo
login fija la cookie.** A partir de ahí, normal. Ningún dato de apps/plantillas/
keys se ve afectado (viven en Postgres, independientes de la sesión).

Orden de despliegue recomendado: **migración `00008` → API → SPA**. La API nueva
es compatible hacia atrás (un login viejo simplemente no tiene cookie y se
renueva al re-loguear).

## B2 · Atomicidad del guardado de aplicación — pendiente

**Dónde:** `web/src/pages/AppDetail.tsx` (`AppHeader.save`).

`save()` hace dos escrituras a endpoints distintos —`PATCH /admin/applications/:id`
(nombre + estado) y `PUT /admin/applications/:id/quota` (cuotas)— no atómicas.

**Mitigado (Bloque A):** `reload()` en `finally` re-sincroniza la cabecera con la
BD, así no muestra datos que ya no coinciden. Resuelve la inconsistencia
**visible**, no la atomicidad real.

**Arreglo propuesto (backend):** un endpoint combinado que actualice
nombre/estado/cuotas en **una** transacción. Riesgo bajo si se **añade** un
endpoint nuevo (no rompe los actuales); el frontend migra cuando exista.
