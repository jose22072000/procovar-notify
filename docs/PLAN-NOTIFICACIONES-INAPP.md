# Plan — Notificaciones in-app en todo el ecosistema QB

Objetivo: que el usuario vea **sus** notificaciones (campana/bandeja) en **cualquier
app** del sistema (qb-auth, qb-panel, qb-booking), que al pulsarlas vaya al sitio
correcto **sin salir de la app en la que está**, y que **solo vea lo suyo**.

## 0. Reglas duras (no negociables)

1. **UNA sola aplicación** en qb-notify para todo el ecosistema. Si cada micro
   tuviera su propia app, la bandeja del usuario saldría **fragmentada** (el inbox
   se consulta por `application_id` + `userId`). Ya está creada.
2. **Aislamiento entre usuarios.** `GET /v1/inbox?userId=X` acepta **cualquier**
   userId: la API key es de la *aplicación*, no del usuario. Por tanto:
   - La key `read` **NUNCA** llega al navegador. Vive solo en el **servidor** de
     cada frontend (sin prefijo `NEXT_PUBLIC_`).
   - El `userId` sale **siempre de la sesión autenticada**, nunca del cliente. El
     navegador pide `/api/notifications/inbox` **sin parámetros**.
   - `GET /v1/notifications/{id}` y `POST /{id}/read` van scopeados solo por
     aplicación → antes de devolver o marcar, el proxy **verifica que
     `recipientUserId` == usuario de la sesión**. Si no → 403.
3. **Permiso mínimo por key**: qb-back solo `send`; qb-panel y qb-booking solo
   `read`; qb-auth `send`+`read` (es el único que emite *y* lee).

## 1. Cómo funciona IN_APP en qb-notify (verificado en el código)

- La **fila de `notifications` ES la entrada de bandeja**; el sender es un no-op.
- Estado: `SENT` = sin leer · `READ` = abierta. **No hay filtro `unreadOnly`** →
  el badge cuenta `status=SENT`.
- El inbox devuelve **`payload` crudo**: **no** devuelve title/body renderizado ni
  campo `action_url`. → **Todo lo que se muestra y el deep-link van en el
  `payload`**, que controlamos nosotros. La plantilla IN_APP debe existir (si no,
  el envío falla) pero su render se descarta: puede ser trivial.
- `recipientUserId` es un texto opaco → usamos **el id de usuario de qb-auth (SSO)**
  en todas las apps, así todas leen las mismas filas.
- El estado de leído es de la **fila** → marcar leída en una app la marca en todas
  (deseable: la lees una vez).

## 2. Catálogo de tipos IN_APP

| `type` | Rol | Cuándo | Payload propio |
|---|---|---|---|
| `reservation_hold_active` | cliente | El cliente inicia la reserva y hay hold con tiempo | `expiresAt`, `resumeToken` |
| `reservation_confirmed` | cliente | Pago OK / reserva confirmada | `code` |
| `reservation_cancelled` | cliente | Cancelada (por el usuario o por expirar el hold) | `reason` |
| `reservation_refund` | cliente | Se crea la solicitud de reembolso con elegibilidad | `refundableAmount`, `chargeAmount`, `currency` |
| `invoice_issued` | cliente | Se emite la factura | `invoiceId`, `amount`, `currency` |
| `owner_reservation_created` | **owner** | Un cliente reserva en **su** propiedad | `guestName` |

### Contrato de payload (común a todas)

```json
{
  "role": "client" | "owner",
  "kind": "reservation" | "invoice",
  "reservationId": "8842",
  "invoiceId": "…",          // solo invoice_issued
  "propertyId": "7",         // clave para el permiso del owner
  "propertyName": "Hotel X",
  "title": "Reserva confirmada",
  "body": "Tu reserva en Hotel X está confirmada.",
  "checkIn": "2026-08-01",
  "checkOut": "2026-08-04"
}
```

`role` es lo que permite que, si eres **cliente Y owner**, recibas **una por cada
rol**, distinguibles, y cada una te lleve a su vista.

## 3. Destino del click — lo resuelve la app donde estás

La notificación **no lleva URL absoluta** (sería incorrecta en otra app). Lleva
`reservationId`/`invoiceId` + `role`, y **cada app tiene su resolver local**:

| Estás en | Va a |
|---|---|
| **qb-auth** | `role=client` → `/profile/reservations/{id}` (o `/profile/invoices/{id}`)<br>`role=owner` → `/profile/org-reservations/{id}` |
| **qb-panel** | **Se queda en el panel**: su vista de esa reserva (detalle / calendario) |
| **qb-booking** | No tiene vista de reservas → **redirige** a qb-auth |

qb-auth = **resumen**. qb-panel = **detalle** (calendario, etc.), solo para owner.

## 4. Permisos (quién ve qué)

- **Cliente**: la notificación se emite **a su `userId`**. Solo la ve él.
- **Owner**: se emite **a cada miembro de la organización que tenga permiso sobre
  ESA propiedad**. El emisor (qb-back) resuelve los destinatarios consultando el
  RBAC de qb-auth (`can(rbac, perm, propertyId)` ya existe e idéntico en los 3
  repos). Un miembro sin permiso sobre esa propiedad **no recibe la fila** → no
  puede verla ni aunque adivine el id.

  → El filtrado es **en el emisor**, no en el frontend: así la notificación ni
  siquiera existe en la bandeja de quien no debe verla.

## 5. Fases

### FASE 0 — Dejar qb-notify sano ✅ (hecho)
- Monitor arreglado (le faltaba `API_BASE` al fetch del SSE).
- Nombre y **slug** de la aplicación editables.
- API keys: **nombre**, **scopes elegibles**, **borrado** y **«último uso»** real.
- Alias `/health` para las probes de k8s.

### FASE 1 — Crear los 6 tipos IN_APP en notify
Por cada tipo: una plantilla IN_APP mínima (`{{title}}`/`{{body}}`, su render se
descarta) + un tipo de notificación (channel_route) que la referencia.
`required_variables` mínimos: `title`, `body`, `reservationId`.

### FASE 2 — Emisor en qb-back (`notifications:send`)
- Cliente HMAC (`src/lib/notify/`): firma `METHOD\nPATH\nQUERY\nSHA256(body)\nTS`,
  cabeceras `X-QBN-Key-Id/Timestamp/Signature`. **Fire-and-forget**: un fallo de
  notify **nunca** rompe la reserva. Envío **después** del commit.
- `idempotencyKey = {reservationId}:{type}` → sin duplicados en reintentos.
- Puntos de emisión: creación del hold · confirmación · cancelación (incluido el
  barrido de expiración) · RefundRequest · emisión de factura · y la de owner.
- **Cancelar** la de `reservation_hold_active` cuando el hold se confirma o expira
  (`POST /v1/notifications/{id}/cancel`, scope `send`).
- Resolver destinatarios owner por RBAC (ver Fase 4).

### FASE 3 — Campana en los frontends (`notifications:read`)
- **Proxy server-side** (aquí vive toda la seguridad del punto 0.2):
  - `GET /api/notifications/inbox` → userId **de la sesión** → notify.
  - `POST /api/notifications/{id}/read` → **verifica dueño** → notify.
- Componente campana: badge = nº `status=SENT`; lista renderizada **desde el
  payload**; click → marca leída + navega con el **resolver local**. En móvil,
  drawer (no modal).
- Orden: **qb-auth primero** (tiene el destino de todo), luego qb-panel, luego
  qb-booking.

### FASE 4 — Permisos del owner
Resolver, en qb-back, los miembros con permiso sobre la propiedad, vía RBAC de
qb-auth. Definir el permiso concreto (reusar uno existente de reservas o añadir
`reservation.view` al catálogo).

### FASE 5 — qb-panel: destino del deep-link
Vista de la reserva en el panel (hoy solo hay **lista**, no detalle por id) y/o
salto al calendario.

## 6. Riesgos conocidos

- **El inbox no devuelve título/cuerpo renderizado ni `action_url`** → todo el
  contenido visible va en el `payload`. Si más adelante se quiere renderizar en el
  servidor, sería un cambio pequeño en qb-notify (persistir subject/body o añadir
  columna `action_url`).
- **Sin `unreadOnly` ni endpoint de contador** → el badge cuenta `status=SENT`.
- **Estado de leído compartido** entre apps (una sola fila). Aceptado y deseado.
- Si la key `read` se filtrara al navegador, **se rompe el aislamiento**. Por eso
  la regla 0.2 es innegociable.
