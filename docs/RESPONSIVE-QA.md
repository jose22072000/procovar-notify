# QA Responsive — mobile first

Revisión vista por vista a distintos anchos. Objetivo: que todo se vea bien de
**320px** (móvil pequeño) hasta **1440px** (ancho máximo que usaremos).

## Anchos a revisar

| 320 | 375 | 414 | 768 | 1024 | 1440 |
|-----|-----|-----|-----|------|------|
| móvil pequeño | móvil estándar | móvil grande | tablet | desktop | máx |

Breakpoints de Tailwind (donde cambia el layout): **sm = 640**, **md = 768**, **lg = 1024**.
Por eso conviene mirar también justo en 640 y 768.

## Qué revisar en cada ancho

- Sin **scroll horizontal** ni contenido cortado/desbordado.
- Texto legible, no apretado; nombres largos **truncan** en vez de romper el layout.
- **Tap targets** cómodos (botones/iconos no minúsculos ni pegados).
- Stacking correcto: lo que en desktop va en fila, en móvil se apila si toca.
- Modales: caben en alto (scroll interno) y no quedan pegados a los bordes.
- Espaciados coherentes (no demasiado padding que coma ancho en móvil).

## Convenciones (aplican a todas las vistas)

- **Botones de acción de fila/tabla** (Editar, Probar, Ver, Revocar, Eliminar,
  Gestionar…): el **texto** no aparece hasta **1024px** (lg); por debajo, **solo icono**.
  `<span className="sr-only lg:not-sr-only">…</span>` + `max-lg:w-8 max-lg:px-0` (cuadrado).
- **Botones de alta** (cabecera "Crear/Nueva"): texto desde **768px** (md); por debajo
  solo icono. `sr-only md:not-sr-only` + `max-md:w-9 max-md:px-0`.
- **Botones solo-icono = cuadrados**: en los anchos donde van sin texto, cuadrados
  (`w-8` para `size="sm"` h-8; `w-9` para tamaño default h-9; `px-0`).
- **Estados (`Badge`)**: en **< 768px** se muestran como **palabra en minúscula + color**
  (sin pastilla); a partir de 768 vuelven a ser **pastilla**. Ya lo hace el componente
  `Badge` (`web/src/ui.tsx`), no hay que tocar cada uso.

## Leyenda

`⬜` pendiente · `✅` ok · `🔧` arreglado · `—` no aplica

## Checklist por vista

| # | Vista | 320 | 375 | 414 | 768 | 1024 | 1440 | Notas |
|---|-------|:---:|:---:|:---:|:---:|:----:|:----:|-------|
| 1 | **Login** (`/login`) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | OK a todos los anchos; añadido icono `LogIn` al botón Entrar |
| 2 | **Aplicaciones** (`/apps` = inicio) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Patrón modal (botón "Nueva aplicación" responsive 425/426); lista = tarjetas <500 / tabla ≥500; "Creada" siempre visible; acción solo-icono hasta 768 |
| 3 | **Detalle app — cabecera + navegación** (drawer móvil) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Editar cuadrado solo-icono <sm; avatar reducido; `nombre / slug · estado` (estado en color, trunca respetando botón) |
| 4 | Tab **API Keys** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Tarjetas apiladas (label/valor); Estado+Último uso 50/50; Revocar arriba-dcha; "Crear" solo-icono <768. Tabla ≥640 |
| 5 | Tab **SMTP** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Tarjetas <768 (info arriba, acciones abajo-dcha) / tabla ≥768; "Nueva" solo-icono <768 |
| 6 | Tab **Proveedores** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Tarjetas <768 / tabla ≥768 (Nombre·Canal·Proveedor·Estado·Eliminar); "Nuevo" solo-icono <768 |
| 7 | Tab **Rutas** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Tarjetas <768 / tabla ≥768 (Tipo·Canal·Prioridad·SMTP·Eliminar); chip "por defecto"; "Nueva" solo-icono <768 |
| 8 | Tab **Templates** (lista) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Tarjetas <768 / tabla ≥768 (Clave·Canal·Nombre·Idioma·acciones); chip vN; Guía/Nueva solo-icono <768 |
| 9 | Templates — **Builder** (modal) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Campos de sección 1 col <sm / 2 col ≥sm; fila "Secciones" con wrap; acciones de sección con tooltip |
| 10 | Templates — **Vista previa / JSON** (modales) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Ya responsive: iframe w-full, JSON `<pre>` overflow-auto, modal con márgenes + scroll. Sin cambios |
| 11 | Tab **Notificaciones** (lista + detalle) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | <1024 acordeón (detalle in-line al expandir); ≥1024 lista+panel |
| 12 | Tab **Monitor** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Ya responsive: KPIs grid-cols-2 sm:4 lg:7. Sin cambios |
| 13 | Tab **Métricas** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Ya responsive: KPIs/desglose grid-cols-2 sm:4. Sin cambios |
| 14 | Tab **Recurrentes** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Tarjetas <768 / tabla ≥768 (Nombre·Estado·Template·Canal·Cron·Eliminar); "Nueva" solo-icono <768 |
| 15 | Tab **Webhooks** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Tarjetas <768 / tabla ≥768 (URL·Estado·Eventos·Eliminar); "Nuevo" solo-icono <768 |
| 16 | Tab **Supresiones** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Tarjetas <768 / tabla ≥768 (Destinatario·Motivo·Canal·Suprimido·Quitar) |
| 17 | Tab **Auditoría** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | `<Table>` → tarjetas stacked en móvil; Actor+IP en 50/50 (rt-half); paginación |
| 18 | Tab **Privacidad** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Formularios (retención / derecho al olvido) apilados en móvil (sm:flex-row) |
| 19 | **Usuarios admin** (`/admins`) | ⬜ | ⬜ | ⬜ | ⬜ | ⬜ | ⬜ | |

## Cómo trabajamos

Vamos vista por vista en orden. Para cada una: recorro el código a los anchos de
arriba, marco lo que veo y propongo/aplico arreglos; tú confirmas en el navegador
(DevTools → modo dispositivo) y tachamos la fila. Notas y decisiones quedan en la
columna **Notas**.
