# Guía: conectar una aplicación nueva a QB Notify

Cómo dar de alta un backend (una **Application**) en QB Notify v2, configurarlo de punta a
punta y hacer que envíe su primera notificación. Es la guía de **onboarding**; el detalle
técnico de la API (firma HMAC completa, todos los endpoints, códigos de error) vive en
[`API-INTEGRACION.md`](./API-INTEGRACION.md).

## La idea en 30 segundos

QB Notify es multi-tenant: cada backend que quiera enviar notificaciones es una
**Application**. Toda la configuración (llaves, destinos, plantillas) se hace **una vez**
en la SPA de administración; después, tu backend solo hace peticiones HTTP firmadas a
`/v1` indicando *qué* enviar y *a quién* — QB Notify se encarga del resto (plantilla,
canal, cola, reintentos, auditoría).

```
 Tu backend                       QB Notify                        Destinatario
 ──────────                       ─────────                        ────────────
 POST /v1/notifications  ──────►  API valida y encola
 { type, recipient, payload }         │
                          202 ◄───────┘
                                  Worker: renderiza plantilla,
                                  resuelve destino (SMTP/Twilio/FCM)
                                        │
                                        └──────────────────────────►  email / SMS /
                                                                      push / in-app
 GET /v1/notifications/{id} ────► estado: SENT / FAILED / ...
```

Lo que el backend manda en cada envío es mínimo:

```json
{ "type": "bienvenida", "recipient": { "email": "ana@ejemplo.com" }, "payload": { "nombre": "Ana" } }
```

El `type` (**Tipo de notificación**) ya lleva asociados el canal, la plantilla y el
destino, porque los configuraste antes en la SPA.

## Requisitos previos

- QB Notify corriendo (en dev: `make up` → `make migrate-up && make seed` →
  `make run-api` + `make run-worker`, y la SPA con `npm run dev` en `frontend/`).
- Acceso a la SPA como **SUPER_ADMIN** (seed de dev: `admin@qbnotify.local` /
  `changeme-admin`, en <http://localhost:5173>).
- Para probar email en dev: MailHog ya viene en el compose (UI en <http://localhost:8025>).

---

## Parte A — Configuración en la SPA (una sola vez por app)

### Paso 1 · Crear la Application

**Aplicaciones → Crear.**

- Nombre y slug (p. ej. "Tienda Online" / `tienda-online`).
- Esto crea el tenant: todo lo que configures a partir de aquí queda aislado bajo esta app.
- Entra con **Gestionar** para configurarla; los pasos siguientes son pestañas de ese detalle.

### Paso 2 · Crear la API key

**Pestaña API Keys → Crear API key.**

- Elige los **scopes**:
  - `notifications:send` — crear/cancelar envíos, batch, eventos, fijar preferencias.
  - `notifications:read` — consultar envíos, bandeja in-app, leer preferencias.
  - Para un backend normal, marca ambos.
- Al crearla se muestran el `keyId` público y el **secret una sola vez**. **Cópialo ya** y
  guárdalo en el gestor de secretos de tu backend (variable de entorno, vault…). Si se
  pierde, se revoca la key y se crea otra — el secret no se puede volver a ver.

### Paso 3 · Configurar el destino de salida

Depende del canal que vaya a usar la app (puedes configurar varios):

- **EMAIL** → pestaña **SMTP**: host, puerto, credenciales y remitente (From). Usa el
  botón **Probar** para validar la conexión antes de seguir.
  *En dev*: MailHog — host `localhost`, puerto `1025`, sin usuario/contraseña.
- **SMS** → pestaña **Proveedores**: alta de Twilio (SID, token, número origen).
- **PUSH** → pestaña **Proveedores**: FCM o APNS con sus credenciales.
- **IN_APP** → no necesita destino: es una bandeja interna que tu backend consulta por API.

Las credenciales se guardan cifradas (AES-256-GCM); nadie las vuelve a ver en claro.

### Paso 4 · Crear la plantilla

**Pestaña Templates → crear.**

- Dos modos: **builder** (editor visual por bloques) o **HTML** (código crudo, solo email).
- Define una `key` estable (p. ej. `welcome`) — es lo que referenciará el Tipo de
  notificación. Las ediciones crean versiones nuevas; el envío usa siempre la última activa.
- Usa variables `{{nombre}}`, `{{empresa}}`… en el contenido: se rellenan con el `payload`
  de cada envío.
- **Preview** con un payload de prueba antes de guardar.
- Puedes crear variantes por idioma (`locale`); el envío elige según el `locale` pedido,
  con fallback a `en`.

### Paso 5 · Crear el Tipo de notificación

**Pestaña Tipos de notificación → crear.** Es la pieza que une todo:

> **Tipo** = nombre único (p. ej. `bienvenida`) + canal (`EMAIL`) + plantilla (`welcome`) + destino (el SMTP del paso 3)

Ese **nombre** es exactamente el `type` que tu backend mandará en cada
`POST /v1/notifications`. Crea un Tipo por cada clase de envío de tu app
(`bienvenida`, `factura`, `reset-password`, `alerta-stock`…).

Con esto la configuración está completa. Todo lo que sigue lo hace tu backend por API.

---

## Parte B — Integración desde tu backend

### Paso 6 · Firmar las peticiones (HMAC)

Toda petición a `/v1` lleva tres cabeceras calculadas con el secret de la API key:

| Cabecera | Valor |
|---|---|
| `X-QBN-Key-Id` | El `keyId` del paso 2 |
| `X-QBN-Timestamp` | Epoch Unix en segundos |
| `X-QBN-Signature` | HMAC-SHA256 hex de: `METHOD \n PATH \n QUERY \n SHA256(BODY) \n TIMESTAMP` |

Helper mínimo en Node (versiones en Python y curl en
[`API-INTEGRACION.md` §2](./API-INTEGRACION.md)):

```js
const crypto = require('crypto');

function qbnHeaders({ method, path, query = '', body = '', keyId, secret }) {
  const timestamp = Math.floor(Date.now() / 1000).toString();
  const bodyHash = crypto.createHash('sha256').update(body, 'utf8').digest('hex');
  const stringToSign = [method.toUpperCase(), path, query, bodyHash, timestamp].join('\n');
  const signature = crypto.createHmac('sha256', secret).update(stringToSign).digest('hex');
  return { 'X-QBN-Key-Id': keyId, 'X-QBN-Timestamp': timestamp, 'X-QBN-Signature': signature };
}
```

Ten en cuenta:

- Reloj sincronizado (NTP): desvío máximo 300 s o la firma expira.
- Cada firma vale **una vez** (anti-replay). Para reintentar, regenera timestamp y firma.
- Si la petición lleva query string, se firma la **query canónica** (claves ordenadas,
  codificación estilo Go) — detalle exacto en `API-INTEGRACION.md` §2.1.

### Paso 7 · Smoke test: `whoami`

Antes de nada, comprueba que la firma funciona:

```
GET /v1/whoami  →  200 { "applicationId": "…", "keyId": "…", "scopes": [...] }
```

Si devuelve `401`, revisa el secret, el reloj y la construcción de la cadena canónica.
También puedes probar sin escribir código con la **colección Postman** de
`docs/postman/` (firma automáticamente; ver `API-INTEGRACION.md` §6).

### Paso 8 · Enviar la primera notificación

```js
const body = JSON.stringify({
  type: 'bienvenida',                                    // el Tipo del paso 5
  recipient: { email: 'ana@ejemplo.com', name: 'Ana' },  // según canal: email | phone | push_token
  recipientUserId: 'user-123',                           // tu id de usuario (necesario para IN_APP/preferencias)
  payload: { nombre: 'Ana' },                            // variables de la plantilla
  idempotencyKey: 'bienvenida-user-123',                 // opcional: evita duplicados en reintentos
});

const res = await fetch('http://localhost:8080/v1/notifications', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    ...qbnHeaders({ method: 'POST', path: '/v1/notifications', body, keyId, secret }),
  },
  body,
});
// 202 { "id": "…uuid…", "status": "QUEUED" }
```

La respuesta `202` significa **encolada**, no enviada: el worker la procesa de forma
asíncrona (renderiza, despacha, reintenta con backoff si falla).

El campo de `recipient` que es obligatorio depende del canal del Tipo:
`email` (EMAIL) · `phone` (SMS) · `push_token` (PUSH) · para IN_APP basta `recipientUserId`.

### Paso 9 · Verificar el resultado

- **Por API**: `GET /v1/notifications/{id}` → el `status` pasa de `QUEUED` a `SENT` (o
  `FAILED` con sus intentos). También hay listado con filtros: `GET /v1/notifications?status=SENT&limit=20`.
- **En la SPA**: pestaña **Notificaciones** (historial y detalle con intentos de entrega)
  y **Monitor** (cola en vivo).
- **En dev con email**: el correo aparece en MailHog (<http://localhost:8025>).

---

## Después del onboarding (opcional)

- **Bandeja in-app**: si usas el canal `IN_APP`, tu frontend la consulta vía tu backend con
  `GET /v1/inbox?userId=…` y marca leídas con `POST /v1/notifications/{id}/read`.
- **Preferencias de usuario**: `GET/PUT /v1/users/{userId}/preferences` para opt-in/out por
  canal. QB Notify las respeta automáticamente al enviar.
- **Batch**: `POST /v1/notifications/batch` para envíos masivos (hasta 1000 destinatarios,
  éxito parcial).
- **Eventos de tu proveedor**: si recibes bounces/quejas de tu ESP, reenvíalos a
  `POST /v1/events` para alimentar la suppression list.
- **Webhooks salientes**: en la pestaña **Webhooks** de la app se registra una URL de tu
  backend y QB Notify te avisa (firmado, con reintentos) de los cambios de estado — evita
  hacer polling.
- **Recurrentes**: pestaña **Recurrentes** para envíos programados con cron.

## Problemas comunes

| Síntoma | Causa probable |
|---|---|
| `401 invalid_signature` | Cadena canónica mal construida (query sin ordenar, body distinto al firmado) o secret incorrecto. |
| `401 signature_expired` | Reloj del cliente desviado > 300 s. Sincroniza con NTP. |
| `401 signature_replayed` | Reutilizaste la misma firma (dos peticiones idénticas en el mismo segundo). Regenera timestamp y firma. |
| `403 missing_scope` | La API key no tiene el scope necesario (`notifications:send` para enviar). |
| `404 notification_type_not_found` | El `type` no coincide con ningún Tipo de notificación de **esta** app (revisa nombre exacto o usa el UUID). |
| `202` pero nunca llega a `SENT` | ¿Worker corriendo? ¿El Tipo tiene plantilla y destino asignados? ¿El SMTP/proveedor pasa el botón *Probar*? Revisa el detalle en la pestaña Notificaciones. |
| `FAILED` con "recipient …" | Falta el campo de `recipient` que exige el canal, o el destinatario está en la suppression list. |
| `429` | Rate limit de la key o cuota de la app excedida. |

## Referencias

- [`API-INTEGRACION.md`](./API-INTEGRACION.md) — referencia completa de la API `/v1`:
  firma HMAC al detalle, todos los endpoints, ejemplos en Node/Python/curl, errores.
- `docs/postman/` — colección Postman con firma automática + environment local.
- [`GUIA-FLUJO.md`](./GUIA-FLUJO.md) — recorrido interno de la SPA pantalla por pantalla.
