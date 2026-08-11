# Guía de integración — API pública de QB Notify v2

Documento para los equipos de las **aplicaciones (tenants)** que van a enviar y consultar
notificaciones a través de **QB Notify v2** (el servicio Go). Cubre la autenticación HMAC,
todos los endpoints públicos, ejemplos de firma en varios lenguajes y el uso de la colección
Postman incluida en `docs/postman/`.

> **Nota sobre el prefijo `/v1`:** las rutas de la API pública del v2 cuelgan de `/v1`
> (p. ej. `POST /v1/notifications`). Es el **versionado de la API HTTP**, no la versión del
> proyecto — no tiene relación con el QB Notify v1 legado de Node, que no se usa.

> La API de administración (`/admin`) es de uso exclusivo de la SPA y **no** se documenta
> aquí: los backends consumidores solo usan `/v1`.

---

## 1. Conceptos

- Cada backend consumidor es una **Application** (tenant). Un super-admin la da de alta en
  la SPA y le crea una o más **API keys**.
- Una API key tiene un `keyId` público (p. ej. `demo-key`), un **secreto** (se muestra una
  sola vez al crearla) y **scopes**:
  - `notifications:read` — listar/consultar notificaciones, bandeja in-app, leer preferencias.
  - `notifications:send` — crear/cancelar notificaciones, batch, eventos, fijar preferencias.
- Los envíos se identifican por un **Tipo de notificación** (configurado en la SPA), que ya
  lleva asociados el canal, la plantilla y el destino (SMTP o proveedor). El cliente solo
  manda `type` + destinatario + variables.
- Canales soportados: `EMAIL`, `SMS`, `PUSH`, `IN_APP` (bandeja interna consultable por API).

**Base URL**: en desarrollo `http://localhost:8080`. En producción, la URL del despliegue de
la API (sin prefijo de ruta adicional; si hubiera un prefijo, entraría en la firma).

---

## 2. Autenticación HMAC

Cada petición a `/v1` se firma con **HMAC-SHA256** usando el secreto de la API key y se
envían tres cabeceras:

| Cabecera | Valor |
|---|---|
| `X-QBN-Key-Id` | El `keyId` público de la API key |
| `X-QBN-Timestamp` | Epoch Unix **en segundos** (entero) |
| `X-QBN-Signature` | HMAC-SHA256 en hexadecimal (minúsculas) de la cadena canónica |

### 2.1 Cadena canónica (string to sign)

```
METHOD \n PATH \n QUERY_CANONICA \n SHA256_HEX(BODY) \n TIMESTAMP
```

- **METHOD**: verbo HTTP en mayúsculas (`GET`, `POST`, `PUT`…).
- **PATH**: la ruta tal cual (`/v1/notifications`), sin query string.
- **QUERY_CANONICA**: los parámetros de query con **claves ordenadas alfabéticamente** y
  codificación estilo `application/x-www-form-urlencoded` de Go (`url.Values.Encode()`):
  espacio → `+`, resto de caracteres reservados en `%XX` mayúsculas; se conservan sin
  codificar `A-Z a-z 0-9 - _ . ~`. **Cadena vacía** si no hay query.
- **SHA256_HEX(BODY)**: hash SHA-256 en hex del cuerpo crudo (para `GET`/sin cuerpo, el
  hash de la cadena vacía: `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`).
- **TIMESTAMP**: el mismo valor enviado en `X-QBN-Timestamp`.

La firma es `hex(HMAC-SHA256(secreto, cadena))`.

### 2.2 Ventana temporal y anti-replay

- El timestamp puede desviarse como máximo `HMAC_TIMESTAMP_SKEW_SECONDS` (default **300 s**)
  del reloj del servidor; fuera de la ventana → `401 signature_expired`. Mantén el reloj del
  cliente sincronizado (NTP).
- Cada firma solo puede usarse **una vez** (guard anti-replay en Redis con TTL 2×ventana).
  Reenviar la misma petición con la misma firma → `401 signature_replayed`. Como la firma
  cubre método, path, query, body y timestamp, dos peticiones idénticas en el **mismo
  segundo** colisionan: para reintentos, regenera timestamp y firma.
- Rate limit por API key; al excederlo → `429`.

### 2.3 Ejemplo en Node.js

```js
const crypto = require('crypto');

// Codificación de query equivalente a url.Values.Encode() de Go.
function goQueryEscape(s) {
  return encodeURIComponent(s)
    .replace(/[!'()*]/g, (c) => '%' + c.charCodeAt(0).toString(16).toUpperCase())
    .replace(/%20/g, '+');
}

function canonicalQuery(params = {}) {
  return Object.keys(params)
    .sort()
    .map((k) => `${goQueryEscape(k)}=${goQueryEscape(String(params[k]))}`)
    .join('&');
}

function qbnHeaders({ method, path, query = '', body = '', keyId, secret }) {
  const timestamp = Math.floor(Date.now() / 1000).toString();
  const bodyHash = crypto.createHash('sha256').update(body, 'utf8').digest('hex');
  const stringToSign = [method.toUpperCase(), path, query, bodyHash, timestamp].join('\n');
  const signature = crypto.createHmac('sha256', secret).update(stringToSign).digest('hex');
  return {
    'X-QBN-Key-Id': keyId,
    'X-QBN-Timestamp': timestamp,
    'X-QBN-Signature': signature,
  };
}

// --- Enviar una notificación ---
const BASE = 'http://localhost:8080';
const keyId = 'demo-key';
const secret = process.env.QBN_SECRET;

const body = JSON.stringify({
  type: 'bienvenida',
  recipient: { email: 'ana@ejemplo.com', name: 'Ana' },
  recipientUserId: 'user-123',
  payload: { nombre: 'Ana' },
});

const res = await fetch(`${BASE}/v1/notifications`, {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    ...qbnHeaders({ method: 'POST', path: '/v1/notifications', body, keyId, secret }),
  },
  body,
});
console.log(res.status, await res.json()); // 202 { id, status }

// --- Listar con query (la query firmada debe ser la canónica) ---
const params = { limit: 20, status: 'SENT' };
const query = canonicalQuery(params); // "limit=20&status=SENT"
const url = `${BASE}/v1/notifications?${query}`;
const res2 = await fetch(url, {
  headers: qbnHeaders({ method: 'GET', path: '/v1/notifications', query, keyId, secret }),
});
```

> Consejo: construye la URL final **con la misma query canónica que firmas** (misma
> ordenación y codificación); así cliente y servidor calculan la firma sobre lo mismo.

### 2.4 Ejemplo en Python

```python
import hashlib, hmac, json, time
from urllib.parse import quote

import requests

BASE = "http://localhost:8080"
KEY_ID = "demo-key"
SECRET = b"demo-secret-please-change"


def _esc(s: str) -> str:
    # Equivalente a url.QueryEscape de Go (espacio -> '+').
    return quote(str(s), safe="-_.~").replace("%20", "+")


def canonical_query(params: dict) -> str:
    return "&".join(f"{_esc(k)}={_esc(v)}" for k, v in sorted(params.items()))


def qbn_headers(method: str, path: str, query: str = "", body: bytes = b"") -> dict:
    ts = str(int(time.time()))
    body_hash = hashlib.sha256(body).hexdigest()
    string_to_sign = "\n".join([method.upper(), path, query, body_hash, ts])
    sig = hmac.new(SECRET, string_to_sign.encode(), hashlib.sha256).hexdigest()
    return {"X-QBN-Key-Id": KEY_ID, "X-QBN-Timestamp": ts, "X-QBN-Signature": sig}


body = json.dumps({
    "type": "bienvenida",
    "recipient": {"email": "ana@ejemplo.com", "name": "Ana"},
    "payload": {"nombre": "Ana"},
}).encode()

r = requests.post(
    f"{BASE}/v1/notifications",
    data=body,
    headers={"Content-Type": "application/json",
             **qbn_headers("POST", "/v1/notifications", body=body)},
)
print(r.status_code, r.json())
```

### 2.5 Ejemplo con curl + openssl

```bash
BASE_URL="http://localhost:8080"
KEY_ID="demo-key"
SECRET="demo-secret-please-change"

BODY='{"type":"bienvenida","recipient":{"email":"ana@ejemplo.com"},"payload":{"nombre":"Ana"}}'
TS=$(date +%s)
BODY_HASH=$(printf '%s' "$BODY" | sha256sum | cut -d' ' -f1)
STRING_TO_SIGN=$(printf 'POST\n/v1/notifications\n\n%s\n%s' "$BODY_HASH" "$TS")
SIG=$(printf '%s' "$STRING_TO_SIGN" | openssl dgst -sha256 -hmac "$SECRET" | sed 's/^.* //')

curl -s -X POST "$BASE_URL/v1/notifications" \
  -H "Content-Type: application/json" \
  -H "X-QBN-Key-Id: $KEY_ID" \
  -H "X-QBN-Timestamp: $TS" \
  -H "X-QBN-Signature: $SIG" \
  -d "$BODY"
```

---

## 3. Formato de errores

Todos los errores son `application/problem+json` (RFC 7807) con un `code` de máquina y un
`traceId` para soporte:

```json
{
  "type": "about:blank",
  "title": "Unauthorized",
  "status": 401,
  "detail": "Invalid or missing request signature",
  "code": "invalid_signature",
  "traceId": "d2f1c0..."
}
```

Códigos habituales: `invalid_signature`, `signature_expired`, `signature_replayed`,
`key_revoked`, `key_expired`, `missing_scope` (403), `notification_type_not_found` (404),
`missing_fields` / validaciones (400), `429` por rate limit o cuota.

---

## 4. Endpoints

Todos bajo `{{baseUrl}}/v1`, autenticados por HMAC. Scope requerido entre paréntesis.

### 4.1 Identidad

#### `GET /v1/whoami` (cualquier key válida)

Devuelve la identidad del tenant — útil como smoke test de la firma:

```json
{ "applicationId": "…uuid…", "keyId": "demo-key", "scopes": ["notifications:read", "notifications:send"] }
```

### 4.2 Envío

#### `POST /v1/notifications` (`notifications:send`)

Crea y encola una notificación. Respuesta `202 Accepted` — el envío real lo hace el worker
de forma asíncrona.

```json
{
  "type": "bienvenida",
  "recipient": { "email": "ana@ejemplo.com", "name": "Ana" },
  "recipientUserId": "user-123",
  "payload": { "nombre": "Ana", "empresa": "Acme" },
  "priority": "NORMAL",
  "locale": "es",
  "idempotencyKey": "pedido-8842-confirmacion",
  "scheduledAt": "2026-07-04T09:00:00Z",
  "attachments": [
    { "filename": "factura.pdf", "contentType": "application/pdf", "url": "https://…" }
  ]
}
```

| Campo | Oblig. | Notas |
|---|---|---|
| `type` | sí* | **Nombre o UUID del Tipo de notificación** (configurado en la SPA). Deriva canal, plantilla y destino. |
| `templateKey` + `notificationType` + `channel` | sí* | **Modo legado**: alternativa a `type` si se quiere indicar plantilla y canal explícitos. |
| `recipient` | según canal | Objeto JSON: `email` (EMAIL), `phone` (SMS), `push_token` (PUSH), `name` opcional. Si falta el campo que exige el canal, el envío falla en el worker. |
| `recipientUserId` | IN_APP | Id del usuario en **tu** sistema; necesario para la bandeja in-app y las preferencias. |
| `payload` | no | Variables para la plantilla (`{{nombre}}`, …). |
| `priority` | no | `LOW` \| `NORMAL` (default) \| `HIGH` \| `URGENT`. Cualquier otro valor se normaliza a `NORMAL`. |
| `locale` | no | Idioma preferido; fallback: locale pedido → `en` → cualquier variante activa. |
| `idempotencyKey` | no | Reintentos seguros: la misma key no crea duplicados. |
| `scheduledAt` | no | RFC3339; programa el envío a futuro. |
| `attachments` | no | Lista de adjuntos: inline (`contentBase64`) o por `url` (sujeto a allowlist SSRF del servidor). |

\* Se exige `type` **o** el trío legado.

**Respuesta** `202`:

```json
{ "id": "b9c2…uuid…", "status": "QUEUED" }
```

#### `POST /v1/notifications/batch` (`notifications:send`)

Envío masivo (misma plantilla/canal, hasta **1000** destinatarios) con **éxito parcial**:

```json
{
  "templateKey": "welcome",
  "notificationType": "bienvenida",
  "channel": "EMAIL",
  "priority": "NORMAL",
  "items": [
    { "recipient": { "email": "a@ejemplo.com" }, "payload": { "nombre": "A" } },
    { "recipient": { "email": "b@ejemplo.com" }, "payload": { "nombre": "B" }, "idempotencyKey": "b-001" }
  ]
}
```

**Respuesta** `200`:

```json
{ "created": ["uuid1", "uuid2"], "failed": [ { "index": 3, "error": "recipient email suppressed" } ] }
```

#### `POST /v1/notifications/{id}/cancel` (`notifications:send`)

Cancela una notificación aún no enviada (pendiente/programada). Devuelve la notificación.

### 4.3 Consulta

#### `GET /v1/notifications` (`notifications:read`)

Listado paginado por cursor. Query params (todos opcionales):

| Param | Valores |
|---|---|
| `status` | `PENDING`, `QUEUED`, `PROCESSING`, `RETRY`, `SENT`, `DELIVERED`, `READ`, `FAILED`, `CANCELLED` |
| `channel` | `EMAIL`, `SMS`, `PUSH`, `IN_APP` |
| `recipientUserId` | id de usuario del tenant |
| `limit` | entero |
| `cursor` | el `nextCursor` de la página anterior |
| `startDate` / `endDate` | RFC3339 |

**Respuesta** `200`:

```json
{
  "data": [
    {
      "id": "…", "templateKey": "welcome", "notificationType": "bienvenida",
      "channel": "EMAIL", "status": "SENT", "queueState": "completed",
      "recipientUserId": "user-123", "recipient": { "email": "…" },
      "payload": { "nombre": "Ana" }, "priority": "NORMAL", "retryCount": 0,
      "createdAt": "2026-07-03T10:00:00Z", "sentAt": "2026-07-03T10:00:02Z"
    }
  ],
  "nextCursor": "…o null…"
}
```

#### `GET /v1/notifications/{id}` (`notifications:read`)

Detalle de una notificación (misma forma que los items del listado).

#### `POST /v1/notifications/{id}/read` (`notifications:read`)

Marca una notificación (típicamente `IN_APP`) como leída. Devuelve la notificación.

#### `GET /v1/inbox?userId=…` (`notifications:read`)

Bandeja **in-app** de un usuario: equivale al listado filtrado por `channel=IN_APP` y
`recipientUserId=userId`. `userId` es obligatorio. Acepta el resto de filtros/paginación
del listado.

### 4.4 Preferencias de usuario

Preferencias opt-in/out por canal de cada usuario final del tenant. Defaults si nunca se
fijaron: `email: true, push: true, sms: false, inApp: true`. El servicio las respeta al
enviar (opt-out ⇒ la notificación no se envía).

- `GET /v1/users/{userId}/preferences` (`notifications:read`)
- `PUT /v1/users/{userId}/preferences` (`notifications:send`)

```json
{ "email": true, "push": true, "sms": false, "inApp": true }
```

### 4.5 Eventos de proveedor

#### `POST /v1/events` (`notifications:send`) — respuesta `202`

Si tu backend recibe webhooks de tu proveedor de email/SMS (bounces, quejas, entregas),
reenvíalos normalizados para que QB Notify mantenga la **suppression list** y confirme
entregas:

| `type` | Campos requeridos | Efecto |
|---|---|---|
| `bounce` / `complaint` | `channel`, `recipient` (+ `reason` opcional) | Añade el destinatario a la suppression list (no se le volverá a enviar por ese canal). |
| `delivered` | `notificationId` | Marca la notificación como `DELIVERED`. |
| otros (`opened`, …) | — | Aceptados sin efecto (por ahora). |

```json
{ "type": "bounce", "channel": "EMAIL", "recipient": "rebota@ejemplo.com", "reason": "mailbox full" }
```

```json
{ "type": "delivered", "notificationId": "b9c2…uuid…" }
```

---

## 5. Webhooks salientes (QB Notify → tu backend)

Además de consultar por API, la SPA permite registrar **webhook endpoints** por aplicación:
QB Notify enviará eventos de estado (enviada, fallida, …) firmados con el secreto del
endpoint, con reintentos con backoff y circuit breaker. Pídele al administrador de tu
Application que registre tu URL si prefieres push en lugar de polling.

---

## 6. Colección Postman

En `docs/postman/`:

- `QB-Notify-v2.postman_collection.json` — todos los endpoints públicos, con un
  **pre-request script a nivel de colección** que calcula la firma HMAC automáticamente
  (path, query canónica, hash del body y timestamp) y añade las cabeceras `X-QBN-*`.
- `QB-Notify-v2-local.postman_environment.json` — environment de desarrollo local.

### Uso

1. En Postman: *Import* → arrastra ambos ficheros.
2. Selecciona el environment **QB Notify v2 — local** y rellena:
   - `baseUrl` — `http://localhost:8080` (ya viene puesto).
   - `keyId` / `apiSecret` — tu API key. Con el seed de desarrollo (`make seed`):
     `demo-key` / el valor de `SEED_API_KEY_SECRET` (default `demo-secret-please-change`).
3. Lanza **Identidad → whoami**: si responde `200`, la firma funciona.
4. En **Notificaciones → Crear (por tipo)** ajusta la variable `notificationType` (nombre o
   UUID de un Tipo de notificación de tu Application) y el `recipient`. Las peticiones de
   creación guardan el `id` devuelto en la variable `notificationId`, que reutilizan
   *Detalle*, *Cancelar*, *Marcar leída* y el evento *delivered*.

Notas:

- El script resuelve las variables `{{…}}` del body **antes** de firmar y reescribe el body
  con ese valor, de modo que lo firmado y lo enviado coinciden (incluidas variables
  dinámicas tipo `{{$guid}}`).
- Si reenvías dos veces la misma petición dentro del mismo segundo, el guard anti-replay
  devolverá `401 signature_replayed`: espera un segundo y reintenta.
- `baseUrl` no debe llevar prefijo de ruta; el path firmado es el que ve el servidor.
