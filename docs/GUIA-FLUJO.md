# Guía: flujo completo + revisión de diseño (SPA)

Guion para recorrer QBNotify de principio a fin **y** revisar el diseño pantalla
por pantalla. En cada paso: (1) qué hace, (2) qué hacer en la UI, (3) revisar el
diseño → me dices cambios → los aplico → marco la pantalla como ✅.

## Entorno (ya levantado)

- SPA: <http://localhost:5173>  ·  Login: `admin@qbnotify.local` / `changeme-admin`
- Buzón de prueba (MailHog): <http://localhost:8025>
- API: <http://localhost:8080>  ·  Worker: en background
- Para apagar todo al final: `make down`

## Cómo trabajamos

Vamos en orden por la tabla de abajo. En cada pantalla me dices: **"ok"** (la
marco ✅ y seguimos) o **"cambia X"** (lo aplico, Vite recarga al instante, y
cuando estés conforme la marco ✅). Las notas de cada pantalla quedan apuntadas
en la sección final.

## Checklist de pantallas

| # | Pantalla / Pestaña | Acción del flujo | Diseño |
|---|--------------------|------------------|--------|
| 1 | **Login** | Entrar con las credenciales | ✅ |
| 2 | **Aplicaciones** | Crear la app demo (ej. "Tienda Online") y entrar con *Gestionar* | ✅ |
| 3 | **Detalle de app · API Keys** | Crear una API key (guardar el secret que se muestra una vez) | ✅ |
| 4 | **SMTP** | Crear conexión al buzón de prueba (MailHog: host `localhost`, puerto `1025`) | ✅ |
| 5 | **Rutas** | Crear ruta por defecto: tipo `transactional` → canal `EMAIL` → el SMTP de MailHog | ✅ |
| 6 | **Templates** | Crear una plantilla con el *builder* (ej. `welcome`) y previsualizar | ✅ |
| 7 | **Envío de prueba** | Enviar una notificación (lo disparo yo por la API `/v1` con la API key) | ✅ |
| 8 | **Notificaciones** | Ver el envío en el historial (estado SENT) y su detalle/intentos | ✅ |
| 9 | **Monitor** | Ver la cola en vivo (encoladas/activas/completadas) | ✅ |
| 10 | **Métricas** | Ver enviadas/fallidas y tasa de error | ✅ |
| 11 | **Recurrentes** | (Opcional) Programar un envío periódico con cron | ✅ |
| 12 | **Webhooks** | (Opcional) Registrar un webhook de estado | ✅ |
| 13 | **Supresiones** | (Opcional) Suprimir un destinatario y ver que el envío se cancela | ✅ |
| 14 | **Privacidad** | (Opcional) Retención de datos y derecho al olvido | ✅ |
| 15 | **Auditoría** | Ver registradas las acciones que hicimos (con IP/usuario) | ✅ |
| 16 | **Menú de usuario / Usuarios admin** | Revisar el menú del navbar y la pantalla de admins | ✅ |

> El **resultado** del flujo (paso 7-8): el correo de bienvenida aparece en
> MailHog (<http://localhost:8025>) y la notificación queda en estado **SENT**.

## Flujo paso a paso (detalle)

1. **Login** → entras al panel.
2. **Aplicaciones** → "Crear" una app (nombre + slug). Es dar de alta un
   tenant/cliente. Luego *Gestionar* para configurarla.
3. **API Keys** → "Crear API key". Aparece el **secret una sola vez** (cópialo).
   Es la llave con la que el backend del cliente pedirá envíos.
4. **SMTP** → alta del buzón de salida. Para la demo usamos MailHog
   (host `localhost`, puerto `1025`, sin usuario/contraseña, From a gusto).
   Botón *Probar* para validar la conexión.
5. **Rutas** → "qué canal usa cada tipo de notificación". Creamos:
   `transactional` + `EMAIL` + el SMTP de MailHog, marcada por defecto.
6. **Templates** → crear plantilla con el builder por secciones (cabecera,
   texto, botón…), con variables `{{...}}`, y *Preview* con un payload de prueba.
7. **Envío de prueba** → con la API key firmamos una petición
   `POST /v1/notifications` (lo hago yo por consola) indicando plantilla,
   destinatario y datos. Queda encolada.
8. **Notificaciones** → el worker la procesa y la envía; la ves como **SENT** y
   puedes abrir su detalle (intentos, eventos, proveedor).
9-15. Resto de pestañas: monitor, métricas, recurrentes, webhooks, supresiones,
   privacidad y auditoría (las recorremos y ajustamos diseño).

## Notas de diseño por pantalla (rediseño shadcn)

Todas las pantallas migradas a **shadcn/ui + Tailwind** (componentes reales:
Button, Input, Label, Select, Textarea, Badge, Dialog, AlertDialog, DropdownMenu,
Avatar, Tabs, Tooltip). Patrón común ("molde"): card + botón "Nuevo" + **modal**
de alta/edición + **estado vacío** con CTA + lista, labels con obligatorio/opcional
y notas cortas, confirmaciones con AlertDialog, autocompletado desactivado.

- **Login** ✅ · **Aplicaciones** ✅ (lista con logo, iconos) · **Cabecera de app** ✅ (modal de edición, cuotas explicadas)
- **API Keys** ✅ · **SMTP** ✅ · **Proveedores** ✅ (campos por proveedor) · **Rutas** ✅ · **Templates + builder** ✅ (guía, preview/JSON separados)
- **Notificaciones** ✅ · **Monitor** ✅ (en vivo) · **Métricas** ✅ · **Recurrentes** ✅ (frecuencia con presets)
- **Webhooks** ✅ · **Supresiones** ✅ · **Auditoría** ✅ · **Privacidad** ✅
- **Usuarios admin** ✅ · **Menú de usuario** ✅
- **Navegación** ✅ (logo = inicio, breadcrumbs, sidebar vertical agrupada con iconos, ancho máx 1440px)

### Decisión de diseño: sin esquinas redondeadas

Todo el diseño es **cuadrado** (sin `border-radius`) en cards, botones, inputs,
modales, dropdowns, badges, avatares y puntos de estado. Centralizado en
`frontend/src/index.css`: tokens `--radius*` a `0` + override `[class*="rounded"] {
border-radius: 0 !important }` (cubre `rounded`/`rounded-full`, que son fijos de
Tailwind). Es **reversible** (restaurar los tokens y quitar el override) y atrapa
cualquier `rounded*` futuro automáticamente.

## Pendiente (próximos días)

1. **Commit/checkpoint** del rediseño de la SPA en `version-2` (hecho al cerrar la sesión; sin push).

2. **Pulido de la SPA** (opcional — lo abordamos a continuación, con Vite en vivo):
   - **Tema oscuro** ✅ — toggle sol/luna en el navbar, bloque `.dark` con tokens, persistencia en `localStorage` + preferencia del sistema, anti-parpadeo en `index.html`.
   - **Responsive** — primer pase hecho: modales con alto máx + scroll, **menú lateral (drawer) en móvil** en el detalle de app, **tablas como tarjetas** en móvil y **botones de acción solo-icono** (`< 768px`). ⚠️ **PENDIENTE: el usuario hará un repaso fino del responsive.** Decisiones aún abiertas:
     - ¿Acortar también los botones **"Crear X"** (API Keys "Crear API key"; CTAs de estado vacío "Crear la primera…", "Crear una plantilla")?
     - ¿Pasar a solo-icono en móvil también **"Gestionar"** (Aplicaciones) y **"Reintentar"/"Cancelar"** (detalle de notificación)?
   - **Tests de la SPA** ✅ — Vitest + React Testing Library + jsdom. Setup con `jest-dom` y mock de `matchMedia`. Tests de humo: `Table` (inyección de `data-label` para móvil) y `ThemeToggle` (alterna/persiste el tema). Scripts: `npm test` y `npm run test:watch`.
   - **Builder de templates: fuente personalizada** ✅ — el builder ofrece web fonts (Google Fonts: Inter, Roboto, Open Sans, Lato, Montserrat, Poppins, Lora, Merriweather) además de las del sistema. El backend inyecta el `<link>` en el `<head>` (solo `https://`, escapado) y el preview (iframe `srcDoc`) la carga. Tests en `builder_test.go` (inyección + guarda no-https).

3. **Endurecimiento del backend** (Paso 2 del plan — `docs/PLAN-SEGUIMIENTO.md` — cada fase con tests propios y commit independiente):
   - **Fase 15** — Anti-SSRF en adjuntos por URL: resolver la IP destino y bloquear loopback/privadas/link-local; allowlist opcional de hosts.
   - **Fase 16** — Caché del token APNS (~1 h) por `(teamId,keyId)`, thread-safe; regenerar al expirar.
   - **Fase 17** — Cobertura E2E extra: firma HMAC, batch parcial, notificación programada (`scheduledAt`).

4. **PR `version-2` → `main`**: lo abre el usuario cuando lo indique (no antes).

5. **Roadmap**: v2.1 ya implementado en backend (webhooks, ingesta/supresión, cuotas, rotación KMS). Más allá quedaría una eventual **v2.2** (APNS HTTP v1/OAuth, anti-SSRF avanzado, etc.).
