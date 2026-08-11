# Checklist de redeploy — Bloque B1 (cookie HttpOnly + revocación)

Cambios a desplegar (rama `staging`, commits `eaddb7d` Bloque A + `f00fd40`
Bloque B1). Ver detalle en `docs/DEUDA-SPA-BLOQUE-B.md`.

## Qué se redeploya

| Componente | ¿Redeploy? | Motivo |
|---|---|---|
| **Migración `00008`** | ✅ Obligatorio | Añade `token_version` a `admin_users`. |
| **API** | ✅ Obligatorio | Nuevo flujo de auth (cookie/refresh/logout/config). Acoplado a la migración (ver abajo). |
| **SPA (frontend)** | ✅ Obligatorio | `client.ts` (refresh por cookie) y `auth.tsx` (logout). El panel viejo espera el refresh en el body y se rompería. |
| **Worker** | ⚪ Opcional | NO toca `admin_users` ni el código de auth: el binario viejo sigue funcionando. Recomendado redeployar por higiene (no mezclar commits), pero no se rompe si se omite. |

## ⚠️ Orden API ↔ migración (acoplados)

El API usa `SELECT *` sobre `admin_users`:
- El **API nuevo necesita** la columna `token_version`.
- El **API viejo se rompe** en login/refresh en cuanto exista la columna
  (desajuste de columnas en el scan).

→ **Aplica la migración y rota el API casi a la vez.** Habrá unos segundos con el
login admin potencialmente caído durante el switch: con un solo admin es
irrelevante (y toca re-loguear una vez de todos modos). El **worker no entra en
este acoplamiento**.

## Pasos

1. **Config del API en prod** (`.env`), según la topología de despliegue:
   - Panel y API en el **mismo dominio** (o un reverse proxy los une):
     ```
     COOKIE_SAMESITE=lax
     COOKIE_SECURE=true
     # COOKIE_DOMAIN vacío (host-only)
     ```
   - Panel y API en **dominios distintos** (ej. panel.x.com / api.y.com):
     ```
     COOKIE_SAMESITE=none
     COOKIE_SECURE=true          # obligatorio (y HTTPS)
     CORS_ALLOWED_ORIGINS=https://panel.tu-dominio.com
     ```
   > `COOKIE_SAMESITE=none` con `COOKIE_SECURE=false` hace fallar el arranque a
   > propósito (los navegadores descartan esa cookie). Si no estás seguro de la
   > topología, empieza con `lax`+`secure=true`; si el login no persiste, pasa a
   > `none` + `CORS_ALLOWED_ORIGINS`.

2. **Aplica la migración** en la BD de prod:
   ```
   make migrate-up        # o: go run ./cmd/migrate up
   make migrate-status    # confirma que 00008 quedó "Applied"
   ```

3. **Redeploy del API** (junto con la migración, hueco mínimo).

4. **Redeploy de la SPA** (`npm run build` y publica `web/dist`).

5. **(Opcional) Redeploy del worker** por consistencia de binarios.

6. **Verifica**: entra al panel → te pedirá login una vez (tu sesión vieja no
   tiene cookie) → a partir de ahí, cookie nueva y funcionamiento normal.

## Impacto en datos

Ninguno. `token_version` es aditiva con `DEFAULT 0`. Aplicaciones, plantillas,
API keys, SMTP, providers y webhooks viven en Postgres, independientes de la
sesión admin. Lo único que pasa es que **te desloguea una vez** al desplegar.

## Rollback

Si algo va mal:
- Revertir binarios (API/SPA) a los anteriores **y** bajar la migración:
  `go run ./cmd/migrate down` (quita la columna). Hazlo en el mismo orden
  inverso (binarios viejos + sin columna) para no dejar el API viejo contra la
  columna nueva.
- No hay pérdida de datos en el rollback (solo se descarta `token_version`).

## Prueba en vivo realizada (dev, 2026-07-09)

Flujo verificado end-to-end contra el servidor real:
login (cookie HttpOnly, sin refresh en el cuerpo) → `/admin/me` con Bearer →
refresh por cookie → logout (cookie borrada) → refresh con la cookie previa =
**401 revocado** → re-login OK.
