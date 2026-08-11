# QB Notify v2 — Diseño técnico (Go + asynq + Redis Sentinel)

> Documento de diseño a nivel senior para la reescritura del microservicio de
> notificaciones. Reemplaza la implementación actual en Node.js/TypeScript
> (Express + BullMQ + Prisma) por un servicio en **Go** con **asynq** sobre
> **Redis Sentinel**, un modelo **multi-tenant** y una **SPA** de administración.

- **Estado**: Diseño aprobado, pendiente de plan de implementación.
- **Fecha**: 2026-06-25
- **Alcance**: Greenfield en Go, esquema nuevo. No se migran datos de la v1
  (el servicio aún no está en producción).
- **Rama de trabajo**: todo el desarrollo de la v2 se realiza en la rama
  `version-2`. **No se hace push directo a `main`**; la integración a `main`
  se hace mediante Pull Request desde `version-2`.

---

## 1. Objetivos y motivación

El servicio pasa de ser una utilidad de envío de emails con un único token y
templates globales, a un **microservicio multi-tenant** consumido por varios
backends. Las metas de la v2:

1. **Multi-tenancy**: cada aplicación backend que consume el servicio es un
   tenant aislado (modelo plano: *Application = tenant = centro de
   autenticación*). Una aplicación gestiona sus propias API keys, conexiones
   SMTP/proveedores, templates y métricas.
2. **Autenticación robusta**: API keys con secret, autenticación por **firma
   HMAC** de la petición (el secret nunca viaja por la red).
3. **Multi-SMTP / multi-proveedor por aplicación**: cada aplicación define sus
   conexiones SMTP y proveedores de canal, y enruta cada *tipo* de notificación
   al proveedor adecuado.
4. **Trazabilidad de origen y destinatario**: cada notificación registra de qué
   aplicación proviene y a qué `userId` (del propio tenant) se notifica. El
   `userId` es **opcional** para soportar destinatarios anónimos (formularios de
   contacto).
5. **Multicanal**: Email (SMTP), Push, SMS e In-App desde el diseño base, con
   una abstracción de canal homogénea.
6. **Alta disponibilidad**: Redis en modo **Sentinel** como broker de colas
   (failover automático), PostgreSQL como fuente de verdad de auditoría.
7. **Interfaz visual**: SPA de administración (React/Next) para gestionar
   aplicaciones, API keys, SMTP/proveedores, templates, y consultar métricas y
   logs. Con roles **super-admin** (plataforma) y **admin por aplicación**.

### No-objetivos (v2)

- Migración de datos de la v1.
- Auto-registro/onboarding self-service público de tenants (alta la realiza un
  super-admin; el self-service queda como evolución futura).

> **Sí es objetivo** un **template builder** visual: la SPA permite construir
> emails a partir de **plantillas predefinidas** (librería global, lo más
> genérica posible) que el tenant clona y personaliza. El builder trabaja con
> **secciones componibles** (cabecera, texto, botón/CTA, imagen, separador, pie…)
> cuyos textos son editables e incluyen **variables Handlebars**. Las variables
> requeridas por cada template se **declaran/derivan** de las secciones y se
> validan por tipo de email. Ver §8.2.

---

## 2. Arquitectura general

```
                         ┌─────────────────────────────┐
   Backends consumidores │  App A   App B   App C  ...  │
   (cada uno = tenant)   └───────────────┬─────────────┘
                                         │  HTTPS + HMAC (API Key/Secret)
                                         ▼
   ┌───────────────────────────────────────────────────────────────┐
   │                        QB Notify v2 (Go)                        │
   │                                                                 │
   │   ┌──────────────┐        encola         ┌──────────────────┐   │
   │   │  API server  │ ────────────────────► │  asynq workers   │   │
   │   │  (chi)       │                        │  (channel send)  │   │
   │   │  REST + Admin│ ◄──── estado ────────  │                  │   │
   │   └──────┬───────┘                        └────────┬─────────┘   │
   │          │                                         │             │
   └──────────┼─────────────────────────────────────────┼────────────┘
              │                                         │
        ┌─────▼─────┐                            ┌──────▼───────┐
        │ PostgreSQL│  (verdad de auditoría)     │ Redis        │
        │           │                            │ Sentinel     │ (broker asynq)
        └───────────┘                            └──────────────┘
              ▲                                         
        ┌─────┴──────┐
        │  SPA Admin │ (React/Next)  ── consume API REST de administración
        └────────────┘
```

### Componentes

- **API server** (`cmd/api`): expone la API REST pública (notificaciones,
  consultas) y la API de administración (consumida por la SPA). Autentica
  peticiones de tenants por HMAC y peticiones de la SPA por sesión/JWT de admin.
- **Worker** (`cmd/worker`): consume tareas de asynq, resuelve la ruta de canal,
  renderiza el template, envía por el proveedor y registra el resultado.
- **Redis Sentinel**: broker de asynq con failover. El cliente Go usa
  `redis.FailoverClient` (master name + sentinels) inyectado en `asynq.RedisFailoverClientOpt`.
- **PostgreSQL**: persiste tenants, credenciales (cifradas), templates,
  notificaciones, intentos de entrega y logs. Es la fuente de verdad; asynq
  conserva solo el estado transitorio de la cola.
- **SPA Admin**: frontend independiente. No comparte despliegue con el binario Go.

### Stack tecnológico

| Capa | Elección | Notas |
|------|----------|-------|
| Lenguaje | Go 1.26+ (última estable: 1.26.4) | |
| Colas | `github.com/hibiken/asynq` | Scheduling, retries, dead-letter, archivado |
| Broker | Redis Sentinel | `asynq.RedisFailoverClientOpt` |
| HTTP | `github.com/go-chi/chi/v5` | Router idiomático sobre `net/http` |
| DB driver | `github.com/jackc/pgx/v5` | Pool nativo, **sin ORM** |
| Acceso a datos | `sqlc` | Queries SQL → código Go type-safe |
| Migraciones | `pressly/goose` | SQL versionado (alt. más potente: Atlas) |
| Templates | `github.com/aymerick/raymond` (Handlebars) | Continuidad con la v1 |
| Validación | JSON Schema (`santhosh-tekuri/jsonschema`) | Variables de template |
| Config | `viper` + variables de entorno | 12-factor |
| Logging | `log/slog` (structured) | |
| Métricas | `prometheus/client_golang` | |
| Cifrado | AES-256-GCM (clave por KMS/env) | Secretos SMTP/proveedor |
| Hash passwords | Argon2id | passwords de `AdminUser` (los secrets de API key van cifrados, no hasheados) |
| Tests | `stretchr/testify` + `testcontainers-go` | Postgres/Redis efímeros |

> **Acceso a datos en Go (sin Prisma)**: la v2 **no usa Prisma** (era el ORM de
> la v1 en Node). Se accede a PostgreSQL directamente con **pgx/v5** y consultas
> SQL escritas a mano, generadas a código type-safe con **sqlc** (sin capa ORM).
> Las migraciones se versionan con **goose** (SQL explícito, embebible en
> `cmd/migrate`); si más adelante se necesita schema-as-code/diffing automático,
> la alternativa recomendada es **Atlas** (`ariga.io/atlas`). Decisión por
> defecto: goose.

---

## 3. Modelo de dominio

Modelo **plano**: `Application` es la raíz del tenant. Todo recurso pertenece a
una `Application` y queda aislado por `application_id`.

### 3.1 Entidades

**Application** — el tenant / centro de autenticación.
```
id            UUID PK
name          text
slug          text unique
status        enum(ACTIVE, SUSPENDED)
created_at, updated_at
```

**ApiKey** — credencial de un backend para consumir la API pública.
```
id            UUID PK
application_id UUID FK -> applications
key_id        text unique          -- identificador público (viaja en cabecera)
secret_enc    bytea                -- secret cifrado AES-256-GCM (recuperable para verificar HMAC; se muestra en claro una sola vez al crearla)
scopes        text[]               -- p.ej. ["notifications:send", "notifications:read"]
status        enum(ACTIVE, REVOKED)
last_used_at  timestamptz null
expires_at    timestamptz null
created_at
```

**SmtpConnection** — conexión SMTP de una aplicación.
```
id            UUID PK
application_id UUID FK
name          text                 -- alias legible ("transactional", "marketing")
host, port    text / int
username      text
password_enc  bytea                -- AES-256-GCM
from_email    text
from_name     text
secure        bool                 -- TLS/STARTTLS
status        enum(ACTIVE, DISABLED)
created_at, updated_at
@@unique(application_id, name)
```

**ChannelProvider** — proveedor para canales no-email (Push/SMS).
```
id            UUID PK
application_id UUID FK
channel       enum(PUSH, SMS, IN_APP)
provider      enum(FCM, APNS, TWILIO, ...)  -- IN_APP no requiere proveedor externo
name          text
config_enc    bytea                -- credenciales/cfg cifradas (AES-256-GCM)
status        enum(ACTIVE, DISABLED)
@@unique(application_id, name)
```

**ChannelRoute** — resuelve "qué proveedor usa cada tipo de notificación".
```
id              UUID PK
application_id   UUID FK
notification_type text             -- categoría libre del tenant: "transactional", "marketing", "alert"...
channel         enum(EMAIL, PUSH, SMS, IN_APP)
smtp_connection_id UUID FK null    -- si channel = EMAIL
channel_provider_id UUID FK null   -- si channel != EMAIL e != IN_APP
priority        int                 -- orden de fallback (menor = primario); ver §7
is_default      bool
@@unique(application_id, notification_type, channel, priority)
```

**Template** — plantilla por aplicación y canal, construida con el template
builder.
```
id              UUID PK
application_id   UUID FK
key             text                -- identificador estable usado por el backend
channel         enum(EMAIL, PUSH, SMS, IN_APP)
base_template_id UUID FK null       -- BaseTemplate del que se clonó (origen)
name, description text
subject         text null           -- aplica a EMAIL / PUSH title (admite variables)
structure       jsonb               -- definición del builder: secciones ordenadas (ver §8.2)
body            text                -- HTML Handlebars compilado desde `structure` (cache de render)
required_variables jsonb            -- JSON Schema derivado de las variables usadas en las secciones
locale          text                -- i18n: idioma del template ("es", "en"...); varias variantes por `key`
version         int                 -- se incrementa al editar; histórico opcional
is_active       bool
created_at, updated_at
@@unique(application_id, key, locale, version)
```

**BaseTemplate** — librería **global** (no por tenant) de plantillas predefinidas
genéricas que el tenant clona como punto de partida.
```
id              UUID PK
key             text unique         -- "welcome", "password-reset", "invoice", "contact-form"...
channel         enum(EMAIL, PUSH, SMS, IN_APP)
category        text                -- "transactional", "marketing", "system"...
name, description text
structure       jsonb               -- secciones por defecto (textos genéricos editables)
suggested_variables jsonb           -- variables típicas sugeridas para este tipo
is_active       bool
created_at, updated_at
```

**Notification** — solicitud de notificación.
```
id              UUID PK
application_id   UUID FK
template_key    text
template_version int null           -- null = última activa
notification_type text              -- usado para resolver ChannelRoute
channel         enum(EMAIL, PUSH, SMS, IN_APP)
recipient_user_id text null         -- userId DEL TENANT (opcional: forms de contacto)
recipient       jsonb               -- {email?, phone?, push_token?, name?}
payload         jsonb               -- variables para el template
attachments     jsonb null          -- [{filename, contentType, url|contentBase64}]
locale          text null           -- idioma solicitado; selecciona variante del template
priority        enum(LOW, NORMAL, HIGH, URGENT)
status          enum(PENDING, QUEUED, PROCESSING, SENT, DELIVERED, READ, FAILED, CANCELLED)
idempotency_key text null
scheduled_at    timestamptz null
expires_at      timestamptz null
max_retries     int
retry_count     int
sent_at, delivered_at, read_at, failed_at  timestamptz null
created_at, updated_at
@@index(application_id, status)
@@index(application_id, recipient_user_id)
@@unique(application_id, idempotency_key)   -- idempotencia por tenant
```

**DeliveryAttempt** — cada intento de envío (sustituye/complementa la tabla
`notification_tasks` de la v1; el job vive en asynq).
```
id              UUID PK
notification_id  UUID FK
attempt_number  int
asynq_task_id   text
status          enum(PROCESSING, SUCCESS, FAILED, RETRY)
provider_ref    text null           -- id del mensaje en el proveedor
error_code      text null
error_message   text null
started_at, finished_at timestamptz
```

**NotificationLog** — eventos de auditoría (created, queued, sent, delivered,
read, failed, retry...).
```
id              UUID PK
application_id   UUID FK
notification_id  UUID FK null
event           text
message         text null
details         jsonb null
created_at
```

**AdminUser** — usuario de la SPA de administración.
```
id              UUID PK
email           text unique
password_hash   text                -- Argon2id
role            enum(SUPER_ADMIN, APP_ADMIN)
application_id   UUID FK null        -- null para SUPER_ADMIN; obligatorio para APP_ADMIN
status          enum(ACTIVE, DISABLED)
last_login_at   timestamptz null
created_at, updated_at
```

**UserNotificationPreference** — preferencias/opt-out por usuario del tenant.
```
id              UUID PK
application_id   UUID FK
user_id         text                -- userId del tenant
email_enabled   bool
push_enabled    bool
sms_enabled     bool
in_app_enabled  bool
updated_at
@@unique(application_id, user_id)
```

**AdminAuditLog** — auditoría de acciones en el panel de administración.
```
id              UUID PK
actor_admin_id  UUID FK -> admin_users
application_id   UUID FK null        -- recurso afectado (null = acción global)
action          text                -- "api_key.create", "smtp.update", "template.publish"...
target_type     text
target_id       text null
details         jsonb null
ip, user_agent  text null
created_at
```

### 3.2 Relaciones (resumen)

```
Application 1───∞ ApiKey
Application 1───∞ SmtpConnection
Application 1───∞ ChannelProvider
Application 1───∞ ChannelRoute ──► SmtpConnection | ChannelProvider
Application 1───∞ Template ──► BaseTemplate (origen, opcional)
BaseTemplate (global) 1───∞ Template          ;  librería global, no pertenece a un tenant
Application 1───∞ Notification 1───∞ DeliveryAttempt
Application 1───∞ NotificationLog
Application 1───∞ UserNotificationPreference
Application 1───∞ AdminUser (APP_ADMIN)   ;  SUPER_ADMIN sin application_id
AdminUser   1───∞ AdminAuditLog
```

---

## 4. Autenticación y autorización

### 4.1 API pública (backends consumidores) — HMAC

Cada `ApiKey` tiene un `key_id` público y un `secret` (mostrado una única vez al
crearla; en BD solo el `secret_enc` cifrado). El backend firma cada petición:

```
StringToSign = METHOD + "\n" +
               PATH + "\n" +
               SHA256(body) + "\n" +
               X-QBN-Timestamp

Signature = HMAC-SHA256(secret, StringToSign)
```

Cabeceras de la petición:

```
X-QBN-Key-Id:    <key_id>
X-QBN-Timestamp: <unix seconds>
X-QBN-Signature: <hex(Signature)>
```

El servidor: localiza la `ApiKey` por `key_id`, recupera el `secret` (ver nota),
recalcula la firma y compara en tiempo constante. Rechaza si el timestamp se
desvía > 5 min (anti-replay) o si la firma no coincide.

> **Nota sobre el secret**: como HMAC requiere el secret para recalcular la firma,
> se almacena **cifrado con AES-256-GCM** (`secret_enc`, clave de servicio en
> KMS/env), no como hash irreversible. Argon2id se reserva para passwords de
> admin. El secret se muestra en claro **una sola vez** al crear la API key.

**Scopes**: cada endpoint público exige un scope (`notifications:send`,
`notifications:read`). El middleware valida que la API key lo posea.

**Aislamiento**: toda query se filtra por el `application_id` derivado de la API
key. Un tenant nunca puede leer/escribir recursos de otro.

**Rate limiting**: por `key_id` (token bucket en Redis).

### 4.2 API de administración (SPA) — sesión

- Login con email/password (Argon2id) → JWT de acceso de corta vida + refresh.
- `SUPER_ADMIN`: acceso a todas las aplicaciones y a la gestión de tenants y
  admins.
- `APP_ADMIN`: acceso restringido a su `application_id` (sus API keys, SMTP,
  proveedores, templates, métricas y logs).
- Middleware de autorización por rol + comprobación de `application_id` en cada
  recurso.

---

## 5. API REST

### 5.1 API pública (HMAC)

```
POST   /v1/notifications              Crear y encolar una notificación
POST   /v1/notifications/batch        Envío masivo: varios destinatarios en una llamada
GET    /v1/notifications/:id          Consultar estado de una notificación
GET    /v1/notifications              Listar (filtros: status, type, recipient_user_id, fechas; paginación cursor)
POST   /v1/notifications/:id/cancel   Cancelar si aún no se envió

# Bandeja in-app y preferencias de usuario
GET    /v1/inbox                      Bandeja in-app del usuario (por recipient_user_id)
POST   /v1/notifications/:id/read     Marcar como leída (status READ)
GET    /v1/users/:userId/preferences  Preferencias de notificación del usuario
PUT    /v1/users/:userId/preferences  Actualizar preferencias (opt-out por canal)
```

`POST /v1/notifications` (cuerpo):
```json
{
  "templateKey": "welcome-email",
  "notificationType": "transactional",
  "channel": "EMAIL",
  "recipient": { "email": "user@acme.com", "name": "Jane" },
  "recipientUserId": "u_123",
  "payload": { "firstName": "Jane", "activationUrl": "https://..." },
  "priority": "HIGH",
  "scheduledAt": null,
  "idempotencyKey": "order-4567-welcome"
}
```

Validación: el `payload` se valida contra `required_variables` (JSON Schema) del
template antes de encolar. Si falla → `422`. Idempotencia por
`(application_id, idempotencyKey)`.

### 5.2 API de administración (sesión)

```
POST   /admin/auth/login
POST   /admin/auth/refresh

# Solo SUPER_ADMIN
GET    /admin/applications
POST   /admin/applications
PATCH  /admin/applications/:id
GET    /admin/admin-users
POST   /admin/admin-users

# SUPER_ADMIN (cualquier app) o APP_ADMIN (su app)
GET    /admin/applications/:appId/api-keys
POST   /admin/applications/:appId/api-keys           -> devuelve el secret una sola vez
DELETE /admin/applications/:appId/api-keys/:id       -> revoca

GET    /admin/applications/:appId/smtp
POST   /admin/applications/:appId/smtp
PATCH  /admin/applications/:appId/smtp/:id

GET    /admin/applications/:appId/providers          -> Push/SMS
POST   /admin/applications/:appId/providers

GET    /admin/applications/:appId/routes             -> ChannelRoute
PUT    /admin/applications/:appId/routes

GET    /admin/applications/:appId/templates
POST   /admin/applications/:appId/templates
PATCH  /admin/applications/:appId/templates/:id
POST   /admin/applications/:appId/templates/:id/preview   -> render con payload de prueba

GET    /admin/applications/:appId/notifications      -> tabla + detalle/logs
GET    /admin/applications/:appId/metrics

# Monitor de tareas / cola (§8.3)
GET    /admin/applications/:appId/tasks               -> listado/filtrado de tareas (estado, prioridad, programadas)
GET    /admin/applications/:appId/tasks/summary       -> contadores por estado/prioridad + profundidad de cola
GET    /admin/applications/:appId/tasks/stream        -> SSE de actualizaciones en vivo
POST   /admin/applications/:appId/tasks/:id/retry     -> reintento manual
POST   /admin/applications/:appId/tasks/:id/cancel    -> cancelar programada/encolada
```

### 5.3 Operación / salud

```
GET    /healthz     liveness
GET    /readyz       readiness (DB + Redis)
GET    /metrics      Prometheus
```

### 5.4 Convenciones de API

- **Versionado**: prefijo `/v1`; los cambios incompatibles van a `/v2`.
- **Paginación**: por cursor (`?limit=&cursor=`), respuesta `{ data, nextCursor }`.
- **Errores**: `application/problem+json` (RFC 7807) con `type`, `title`,
  `status`, `detail`, `traceId`.
- **Idempotencia**: cabecera `Idempotency-Key` en los POST de envío.
- **Rate limit**: cabeceras `X-RateLimit-*` en la respuesta.

---

## 6. Procesamiento de notificaciones (asynq)

### 6.1 Flujo

1. **API**: valida HMAC + scope + payload contra el template; si el destinatario
   tiene opt-out del canal (`UserNotificationPreference`) → `CANCELLED` + log sin
   encolar. Si no → persiste `Notification` (`PENDING`) → encola tarea asynq →
   `QUEUED`.
2. **Encolado**: tipo de tarea `notification:send`, payload `{notificationId}`.
   Cola elegida por prioridad (ver colas). `scheduledAt` → `ProcessAt`.
3. **Worker**: carga la `Notification`, resuelve `ChannelRoute`
   `(application, notificationType, channel)` → obtiene `SmtpConnection` o
   `ChannelProvider`. Crea `DeliveryAttempt` (`PROCESSING`).
4. **Render**: compila el `body` Handlebars con el `payload`. Para EMAIL genera
   también subject.
5. **Envío**: dispatcher por canal (`EmailSender`, `PushSender`, `SmsSender`,
   `InAppSender`). Descifra credenciales en memoria.
6. **Resultado**:
   - Éxito → `DeliveryAttempt=SUCCESS`, `Notification=SENT` (+`provider_ref`),
     log `sent`.
   - Fallo recuperable → asynq reintenta con backoff exponencial hasta
     `max_retries`; `DeliveryAttempt=RETRY`.
   - Agotados los reintentos → cola de archivado/dead-letter,
     `Notification=FAILED`, log `failed`.

### 6.2 Colas y prioridades

asynq con colas ponderadas:

```
critical : 6   (priority URGENT)
default  : 3   (priority HIGH/NORMAL)
low      : 1   (priority LOW)
```

### 6.3 Redis Sentinel

```go
redisOpt := asynq.RedisFailoverClientOpt{
    MasterName:    cfg.RedisMasterName,
    SentinelAddrs: cfg.RedisSentinelAddrs, // ["host1:26379", ...]
    Password:      cfg.RedisPassword,
    DB:            cfg.RedisDB,
}
```
Tanto el `asynq.Client` (API) como el `asynq.Server` (worker) usan
`RedisFailoverClientOpt` para failover transparente del master.

### 6.4 Idempotencia y reintentos

- Idempotencia de entrada: `(application_id, idempotencyKey)` evita duplicar la
  `Notification`.
- Reintentos: `max_retries` por notificación, backoff exponencial de asynq.
- Anti-doble-envío: `DeliveryAttempt` + estado de la `Notification` evitan
  reenviar una notificación ya `SENT`.

### 6.5 Notificaciones recurrentes

Para digests/recordatorios periódicos se usan los **periodic tasks** de asynq
(`PeriodicTaskManager`, expresión cron por tenant). Cada disparo genera una
notificación reutilizando el flujo de envío estándar. Semántica fina (catch-up,
zona horaria, solapamiento) → plan de implementación.

---

## 7. Abstracción de canales

Interfaz común; un sender por canal:

```go
type Sender interface {
    Channel() Channel
    Send(ctx context.Context, msg RenderedMessage, route ResolvedRoute) (ProviderRef, error)
}
```

- **EmailSender**: SMTP (`net/smtp` o `go-mail`), credenciales de `SmtpConnection`.
- **PushSender**: FCM/APNs vía `ChannelProvider`.
- **SmsSender**: Twilio (u otro) vía `ChannelProvider`.
- **InAppSender**: persiste la notificación como entrada de bandeja consultable
  por API (sin proveedor externo).

Añadir un canal/proveedor nuevo = implementar `Sender` + registrar en el
dispatcher; el modelo de datos no cambia.

### 7.1 Fallback y resiliencia de proveedor

`ChannelRoute` admite **varias rutas por (tipo, canal)** ordenadas por
`priority`. Si el envío por la ruta primaria falla de forma recuperable, el
worker prueba la siguiente ruta antes de agotar reintentos. Cada `Sender` lleva
un **circuit breaker** por conexión/proveedor: tras N fallos consecutivos se
abre y deriva al fallback, evitando martillear un proveedor caído.

---

## 8. Interfaz visual (SPA Admin)

SPA en **React/Next** que consume la API de administración.

### 8.1 Pantallas

1. **Login** (email/password).
2. **Aplicaciones** (solo super-admin): listado, alta, suspensión.
3. **Detalle de aplicación** (super-admin o admin de esa app):
   - **API Keys**: crear (muestra el secret una sola vez), revocar, ver
     `last_used`.
   - **SMTP**: alta/edición de conexiones (password nunca se muestra tras
     guardar), test de conexión.
   - **Proveedores** (Push/SMS): alta/edición de credenciales.
   - **Rutas de canal**: mapa `tipo de notificación → canal → proveedor/SMTP`.
   - **Templates** (template builder, ver §8.2): listado; crear desde una
     plantilla predefinida o en blanco; editor por secciones; declaración/
     derivación de variables; **preview** con payload de prueba.
   - **Notificaciones**: tabla con filtros (estado, tipo, `recipientUserId`,
     fechas) y detalle con intentos/logs.
   - **Tareas / Cola** (monitor en vivo, ver §8.3): estado del procesamiento por
     aplicación — encoladas, en proceso, programadas, reintentando, completadas
     y fallidas — con tareas programadas a futuro y línea de tiempo por
     notificación.
   - **Métricas**: enviadas/fallidas/pendientes, tasa de error, latencia.
   - **Auditoría**: registro de acciones de admin (`AdminAuditLog`) filtrable.
4. **Admins** (solo super-admin): gestión de `AdminUser` y asignación a apps.

El recipiente `userId` puede no existir (forms de contacto): la UI lo muestra
como "anónimo/contacto" cuando `recipientUserId` es null.

### 8.2 Template builder

Construcción visual de plantillas por **secciones componibles**, no edición de
HTML a mano. Flujo:

1. **Punto de partida**: el admin crea un template eligiendo una **plantilla
   predefinida** de la librería global (`BaseTemplate`: welcome, password-reset,
   invoice, contact-form, …) o partiendo en blanco. Al clonar, `structure` y
   `suggested_variables` se copian como base editable; `base_template_id` queda
   referenciado.
2. **Edición por secciones**: panel con lista ordenable (drag & drop) de
   secciones. Tipos base:
   - `header` (logo/título), `text` (párrafo rico), `button` (CTA con
     `text` + `url`), `image`, `divider`, `spacer`, `footer`.
   - Cada sección expone campos de texto **editables** que admiten **variables
     Handlebars** (`{{firstName}}`, `{{activationUrl}}`…) y condicionales/listas
     (`{{#if}}`, `{{#each}}`).
   - Se añaden, eliminan, duplican y reordenan secciones libremente.
3. **Variables por tipo de email**: el builder **detecta** las variables usadas
   en todas las secciones y en el `subject`, y construye `required_variables`
   (JSON Schema). El admin puede marcar cada variable como requerida/opcional y
   fijar tipo/descripción. Así cada *type of email* declara exactamente las
   variables que necesita.
4. **Preview**: render en vivo con un payload de prueba; avisa si faltan
   variables requeridas. Endpoint `POST /admin/.../templates/:id/preview`.
5. **Compilación**: al guardar, `structure` se compila a `body` (HTML Handlebars
   responsive, con estilos inline para compatibilidad de clientes de correo) y
   se incrementa `version`.

**Formato de `structure`** (ejemplo):
```json
{
  "theme": { "primaryColor": "#0B5", "fontFamily": "Arial, sans-serif" },
  "sections": [
    { "id": "s1", "type": "header", "props": { "logoUrl": "{{logoUrl}}", "title": "{{appName}}" } },
    { "id": "s2", "type": "text",   "props": { "text": "Hola {{firstName}}, te damos la bienvenida." } },
    { "id": "s3", "type": "button", "props": { "text": "Activar cuenta", "url": "{{activationUrl}}" } },
    { "id": "s4", "type": "footer", "props": { "text": "© {{year}} {{appName}}" } }
  ]
}
```

La librería `BaseTemplate` es global y semillada (`db/seeds`); evoluciona aparte
de los tenants. Un super-admin puede ampliarla. El renderizado en el worker (§6)
usa el `body` compilado + `payload`, igual que cualquier template.

### 8.3 Monitor de tareas / cola (por aplicación)

Visión en vivo de cómo avanza el procesamiento, **aislada por `application_id`**.
Permite a cada tenant ver el estado de sus tareas sin acceder al panel global de
operaciones.

- **Resumen de cola**: contadores por **estado de cola de asynq** —`scheduled`,
  `pending`, `active`, `retry`, `archived`, `completed`— y por prioridad
  (`critical`/`default`/`low`). Profundidad y throughput. Estos estados de cola
  se derivan del par (`Notification.status`, `scheduled_at`); no son un enum
  nuevo en BD.
- **Tareas programadas**: notificaciones con `scheduled_at` futuro, ordenadas por
  fecha de ejecución; permite cancelar antes del envío.
- **En proceso / recientes**: tareas activas y su último resultado, con el canal
  y el proveedor/SMTP resuelto.
- **Línea de tiempo por notificación**: secuencia de eventos
  (`created → queued → processing → sent → delivered/read` o `retry → failed`)
  a partir de `DeliveryAttempt` + `NotificationLog`, con código/mensaje de error
  y `provider_ref` de cada intento.
- **Acciones**: reintentar manualmente una tarea fallida y cancelar una
  programada (sujeto a scopes del admin).

**Datos y tiempo real**:
- La vista por tenant se construye desde **PostgreSQL** (`Notification`,
  `DeliveryAttempt`, `NotificationLog`, todos con `application_id`), que es la
  fuente fiable y filtrable por aplicación.
- Los contadores agregados de cola se complementan con la API de inspección de
  **asynq** (profundidad/estados por cola), filtrando por el tenant cuando
  aplique.
- Actualización en vivo vía **SSE** (`GET /admin/applications/:appId/tasks/stream`)
  o *polling* corto; el worker emite eventos de progreso al cambiar el estado de
  un `DeliveryAttempt`.

> `asynqmon` (§12) sigue siendo la herramienta **global** de operaciones para el
> equipo de plataforma; §8.3 es la vista **por aplicación** integrada en la SPA.

---

## 9. Estructura del repositorio (Go)

```
qb-notify/
├── cmd/
│   ├── api/main.go          # API server (chi)
│   ├── worker/main.go       # asynq worker
│   └── migrate/main.go      # goose runner (o Makefile target)
├── internal/
│   ├── config/              # viper + env
│   ├── http/                # router, middlewares (hmac, auth, ratelimit), handlers
│   ├── auth/                # HMAC, JWT admin, scopes
│   ├── domain/              # entidades y lógica de negocio
│   ├── store/               # sqlc generado + repos (pgx)
│   ├── queue/               # cliente/servidor asynq, tipos de tarea
│   ├── channels/            # Sender: email, push, sms, inapp
│   ├── template/            # render Handlebars + validación JSON Schema
│   ├── crypto/              # AES-GCM, Argon2id
│   └── observability/       # slog, prometheus, health
├── db/
│   ├── migrations/          # goose (.sql)
│   └── queries/             # sqlc (.sql)
├── api/openapi.yaml         # contrato de la API
├── web/                     # SPA React/Next (o repo aparte)
├── deploy/                  # docker-compose, k8s, sentinel
├── Makefile
├── go.mod
└── new-version.md
```

---

## 10. Configuración (env)

```env
# Server
APP_ENV=production
API_PORT=8080
WORKER_CONCURRENCY=10

# PostgreSQL
DATABASE_URL=postgres://user:pass@host:5432/qb_notify

# Redis Sentinel (entorno local — accesible solo en la red interna)
REDIS_SENTINELS=10.20.10.61:26379,10.20.10.61:26380,10.20.10.61:26381
REDIS_MASTER_NAME=master
REDIS_PASSWORD=permiso
REDIS_SENTINEL_PASSWORD=permiso
REDIS_DB_DEFAULT=4          # uso general / caché
REDIS_DB_LOCKS=5           # locks distribuidos
REDIS_DB_ASYNQ=10          # colas de asynq
REDIS_PREFIX=qb-notify

# Crypto
SECRET_ENCRYPTION_KEY=        # 32 bytes base64 (AES-256-GCM) para SMTP/provider/secrets
ADMIN_JWT_SECRET=

# Defaults de cola
QUEUE_RETRY_MAX=3
HMAC_TIMESTAMP_SKEW_SECONDS=300
```

---

## 11. Seguridad

- Secretos SMTP/proveedor **cifrados en reposo** (AES-256-GCM).
- Passwords de admin con **Argon2id**.
- HMAC con ventana anti-replay y comparación en tiempo constante.
- Aislamiento estricto por `application_id` en cada query.
- Rate limiting por API key.
- TLS obligatorio en producción; el secret de la API key se muestra una sola vez.
- Auditoría completa en `NotificationLog` (origen = aplicación, destinatario =
  `recipientUserId` o contacto anónimo).
- Secret de API key cifrado AES-256-GCM (`secret_enc`), nunca en claro en BD.
- **Audit log de administración** (`AdminAuditLog`): toda acción sensible del
  panel (crear/revocar API keys, editar SMTP/proveedores, publicar templates)
  queda registrada con actor, IP y user-agent.

### 11.1 Retención de datos y privacidad (PII/GDPR)

- Las notificaciones almacenan PII (`recipient`, `payload`). **Retención
  configurable por tenant**: tras `N` días se **purga/anonimiza** el contenido
  (`payload`, `recipient`) conservando metadatos de auditoría.
- **Derecho al olvido**: proceso/endpoint para borrar las notificaciones de un
  `recipient_user_id` a petición del tenant.
- Datos sensibles cifrados en reposo; logs sin volcar PII en claro.
- Plazos por defecto y anonimización vs. borrado → plan de implementación.

---

## 12. Observabilidad

- **Logs**: `slog` estructurado en JSON, con `application_id`, `notification_id`,
  `task_id` correlacionados.
- **Métricas Prometheus**: notificaciones por estado/canal/tenant, latencia de
  envío, profundidad de cola, reintentos, errores por proveedor.
- **Health**: `/healthz` (liveness), `/readyz` (DB + Redis).
- Dashboard de asynq (`asynqmon`) opcional para inspección de colas.

---

## 13. Estrategia de pruebas

- **Unit**: firma/verificación HMAC, render Handlebars, validación JSON Schema,
  resolución de `ChannelRoute`, cifrado AES-GCM.
- **Integración** (testcontainers): repos sqlc contra Postgres efímero; encolado
  y consumo asynq contra Redis efímero.
- **E2E de API**: ciclo completo `POST /v1/notifications` → worker → estado
  `SENT` con un SMTP de prueba (MailHog/mock).
- **Contrato**: validación de la API contra `openapi.yaml`.

---

## 14. Despliegue

- Dos despliegues desde el mismo binario/imagen: `api` y `worker`
  (`scale` independiente).
- Redis Sentinel con ≥ 3 sentinels y 1 master + réplicas.
- PostgreSQL gestionado o con réplica.
- Migraciones ejecutadas en el arranque/CI (`goose up`).
- SPA servida como estático (CDN/Nginx) o app Next desplegada aparte.

---

## 15. Cambios respecto a la v1

| v1 (Node.js) | v2 (Go) |
|--------------|---------|
| Express + BullMQ + Prisma | chi + asynq + sqlc/pgx |
| Redis simple | Redis **Sentinel** (HA) |
| Único `BEARER_TOKEN` | **API Keys + HMAC** por aplicación |
| SMTP único global (env) | **Multi-SMTP/proveedor por aplicación** + rutas por tipo |
| Templates globales | Templates **por aplicación**, versionados, con JSON Schema |
| Sin tenancy | **Multi-tenant** plano (App = tenant) |
| Sin UI | **SPA de administración** (super-admin + admin por app) |
| `notification_tasks` | `DeliveryAttempt` (estado de cola en asynq) |

---

## 16. Riesgos y decisiones abiertas

- **Almacenamiento del secret de API key**: RESUELTO — cifrado AES-256-GCM
  (`secret_enc`), recuperable para verificar HMAC; Argon2id solo para passwords
  de admin.
- **Versionado de templates**: mantener histórico completo vs. última activa.
  Diseño preparado para histórico (`@@unique(application_id, key, locale, version)`).
- **Proveedores Push/SMS concretos** (FCM/APNs/Twilio): credenciales y SDKs a
  detallar por proveedor en implementación.
- **SPA: repo propio vs. carpeta `web/`**: a decidir según pipeline de CI.
- **Recurrencia**: catch-up, zona horaria y solapamiento de los periodic tasks.
- **i18n**: fallback de idioma cuando no existe variante del template solicitada.
- **Adjuntos**: límite de tamaño y si se aceptan por URL o inline (base64).
- **Bulk send**: tope de destinatarios por llamada y manejo de errores parciales.
- **Retención PII**: plazos por defecto y anonimización vs. borrado.
- **No incluidos (roadmap)**: webhooks de estado al tenant, ingesta de eventos de
  proveedor (bounces/opens), suppression list/unsubscribe, rotación de claves
  KMS y cuotas por tenant. Se evaluarán en una v2.1.
```
