# Guía de operación: dar de alta una app externa y probar la integración

Guía práctica y sin tecnicismos para el administrador de QB Notify. Cubre: qué configurar
en el panel web, qué entregarle al desarrollador de la app externa y cómo probar juntos
que todo funciona (ej.: que al registrarse un usuario le llegue el correo).

La referencia técnica completa para el desarrollador externo es
[`API-INTEGRACION.md`](./API-INTEGRACION.md); la versión más técnica de esta guía es
[`GUIA-CONEXION-APP.md`](./GUIA-CONEXION-APP.md).

---

## Parte 1 — Tú, en el panel de Notify

1. **Crear la aplicación** — Aplicaciones → Crear → nombre y slug (ej. "App X").
   Luego entra con **Gestionar**; todo lo demás son pestañas de ese detalle.

2. **Crear la API key** — pestaña **API Keys** → Crear → marca los dos permisos
   (enviar y consultar). Al crearla se muestran el **Key ID** y el **Secret**.
   ⚠️ **El Secret se muestra una sola vez** — cópialo en ese momento. Eso es lo que
   le darás al desarrollador externo. Si se pierde: se revoca la key y se crea otra.

3. **Configurar el correo de salida** — pestaña **SMTP** → Crear → datos de tu
   proveedor de correo real (host, puerto, usuario, contraseña y el remitente "From"
   que verá la gente). Usa el botón **Probar** hasta que dé OK — sin esto, nada sale.

   > **Puertos SMTP:** con la opción de seguridad activada, usa **465** (SSL) o
   > **587** (STARTTLS) — la app elige el modo de cifrado según el puerto. Sin
   > cifrado (solo pruebas internas): 25/1025. Y ojo con los dedos: `456` no es
   > un puerto de correo — es el typo clásico de `465` y da *i/o timeout*.

4. **Crear la plantilla del correo** — pestaña **Templates** → crea una por cada
   correo (ej. `welcome` para el registro). Donde vayan datos del usuario usa
   variables: `Hola {{nombre}}, bienvenido a {{empresa}}`. Revisa con el Preview.

5. **Crear el Tipo de notificación** — pestaña **Tipos de notificación** → Crear.
   Es la pieza que une todo:
   - **Nombre**: ej. `registro-bienvenida` — esta es la palabra clave que usará el
     desarrollador externo en cada envío.
   - **Canal**: EMAIL.
   - **Plantilla**: la del paso 4.
   - **Destino**: el SMTP del paso 3.

6. **Repite 4 y 5 por cada uso**: `reset-password`, `confirmacion-pedido`, etc.
   Una plantilla + un tipo por cada correo distinto. El desarrollador externo no
   toca configuración: solo usa el nombre del tipo.

---

## Parte 2 — Lo que le entregas al desarrollador externo

Pásale lo siguiente (el Secret por un canal seguro — gestor de contraseñas o similar,
nunca en un chat o correo en claro):

> **Datos de conexión a Notify:**
> - URL: `https://notify-api.divergtech.com`
> - Key ID: `(el del paso 2)`
> - Secret: `(el del paso 2)`
>
> **Tipos de notificación disponibles** (el valor de `type` al enviar) y el
> payload exacto de cada uno — p. ej. con la plantilla "Bienvenida" del sistema:
> - `registro-bienvenida` — variables: `firstName`, `appName`, `actionUrl`, `year`
> - (la lista que hayas creado, cada tipo con sus variables exactas)
>
> **Cómo se envía** — una sola petición HTTP:
>
> ```
> POST https://notify-api.divergtech.com/v1/notifications
> {
>   "type": "registro-bienvenida",
>   "recipient": { "email": "correo@delusuario.com", "name": "Ana" },
>   "payload": { "firstName": "Ana", "appName": "App X", "actionUrl": "https://appx.com/entrar", "year": "2026" }
> }
> ```
>
> Cada petición va **firmada** con el Secret (3 cabeceras `X-QBN-*`). El código de
> firma listo para copiar (Node, Python, curl) está en la sección 2 del documento
> adjunto `API-INTEGRACION.md`.

### El cuerpo de la petición, campo por campo

```json
{
  "type": "registro-bienvenida",
  "recipient": {
    "email": "usuario@ejemplo.com",
    "name": "Ana Pérez"
  },
  "payload": {
    "firstName": "Ana",
    "appName": "App X",
    "actionUrl": "https://appx.com/entrar",
    "year": "2026"
  }
}
```

| Campo | Qué es | ¿Obligatorio? |
|---|---|---|
| `type` | El nombre del **Tipo de notificación** creado en el panel, exacto | Sí |
| `recipient.email` | El correo de quien recibe (`name` es opcional) | Sí para email |
| `payload` | **Las variables de la plantilla**: cada `{{algo}}` de la plantilla se rellena con la clave igual de aquí | Solo si la plantilla usa variables |

La clave: **`payload` no tiene formato fijo — lo definen las variables de la plantilla
concreta**. El ejemplo de arriba corresponde a la plantilla "Bienvenida" que trae el
sistema (`{{firstName}}`, `{{appName}}`, `{{actionUrl}}`, `{{year}}`); si tu plantilla
usa otras variables, el payload cambia con ellas. Los nombres deben coincidir **exactos**
(mayúsculas incluidas: `firstName`, no `firstname`).

**Dónde ver el payload exacto de cada plantilla:** en el panel → Templates → abre la
plantilla → el panel de variables muestra la lista y el JSON de ejemplo. Copia ese JSON
tal cual y pásaselo al desarrollador junto al nombre del tipo — así no hay que adivinar.

Campos opcionales que puede añadir cuando los necesite:

```json
{
  "recipientUserId": "user-123",          // id del usuario en SU sistema (historial por usuario)
  "idempotencyKey": "registro-user-123",  // evita duplicados si reintenta la misma petición
  "scheduledAt": "2026-07-05T09:00:00Z",  // programar el envío a futuro
  "priority": "HIGH"                      // LOW | NORMAL | HIGH | URGENT
}
```

La respuesta buena es `202` con `{ "id": "…", "status": "QUEUED" }` — encolado; el
correo sale en segundos. En la colección Postman, esta petición es **"Crear (por tipo)"**.

**Archivos a adjuntarle** (están en este repo):

- `docs/API-INTEGRACION.md` — la referencia técnica completa.
- `docs/postman/` (2 JSON) — colección Postman que firma sola, para probar sin código.

---

## Parte 3 — Probar la integración

**Prueba 1 · La conexión** (la hace él, 5 minutos):
importa la colección Postman, rellena URL + Key ID + Secret en el environment y lanza
**whoami**. `200` = llave y firma funcionan. `401` = secret mal copiado o el reloj de
su servidor desajustado (debe estar en hora, margen de 5 minutos).

**Prueba 2 · El primer correo** (él dispara, tú verificas):

1. Él lanza `POST /v1/notifications` con `type: registro-bienvenida` y **su propio
   correo** como destinatario (desde Postman o su código).
2. La respuesta debe ser `202` con un `id` — significa "aceptado y en cola".
3. En segundos le llega el correo con tu plantilla.
4. Tú lo ves en el panel → pestaña **Notificaciones**: estado **SENT**. Si sale
   **FAILED**, el detalle dice por qué (casi siempre: SMTP mal configurado o faltó
   el email del destinatario).

**Prueba 3 · El flujo real** (registro en la App X):
él conecta el envío a su código — cuando un usuario se registra en la App X, su
backend hace esa misma petición con el correo del usuario. Prueban registrando una
cuenta de prueba → llega el correo de bienvenida → tú lo ves en Notificaciones.

**Los demás usos son el mismo patrón:** tú creas plantilla + tipo en el panel, le
avisas del nombre y sus variables, y él solo cambia `type` y `payload` en su código.
Las llaves y la configuración no se vuelven a tocar.

---

## Si algo falla, revisa en este orden

1. ¿`whoami` da `200`? Si no: secret o reloj.
2. ¿El envío devolvió `202`? Si no: el `type` no existe en esa app (nombre exacto)
   o falta un campo.
3. ¿Qué dice el detalle en la pestaña **Notificaciones**? Ahí está el motivo real
   de cualquier fallo de envío.
4. ¿El SMTP pasa el botón **Probar**? Si no, nada va a salir (revisa host, puerto
   —587—, usuario y contraseña).
