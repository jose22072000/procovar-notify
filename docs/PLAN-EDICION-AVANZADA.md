# Plan — Edición avanzada de plantillas (modo HTML) + doble enfoque

> Objetivo: mantener el **editor visual (builder)** para quien no sabe de código y
> añadir un **modo HTML avanzado** (HTML/CSS crudo) para quien sí, con la seguridad
> necesaria. Tener las 12 plantillas por defecto **en ambas bibliotecas**.
> Rama de trabajo: `feature/cambios-ui-v2` (o rama nueva). No se hace push hasta indicarlo.

## 0. Decisiones cerradas

- **Roles**: el modo HTML está disponible para **todo admin** (super-admin y admin de app). Sin gate por rol; se **audita** cada edición.
- **Conversión**: `builder → html` cuando el usuario quiera (una vía) **+** botón **"volver al editor visual"** que **descarta los cambios de HTML** y recupera el último estado de bloques. `html → builder` real (conservando el HTML escrito a mano) **no** se soporta.
- **Las 12 por defecto**: en **ambas** bibliotecas — versión `builder` (simplificada, ya sembrada) y versión `html` (fiel, del CSV). Claves: las `html` con sufijo `-html`.
- **Editor**: **CodeMirror** (`@uiw/react-codemirror` + `@codemirror/lang-html`).
- **Canales**: modo HTML **solo `EMAIL`**.

## 1. Principio de diseño

Cada plantilla lleva un campo **`kind`** (`builder` | `html`) que decide editor,
guardado y compilado. **El envío no cambia**: ambos modos terminan produciendo el
`body` (HTML con `{{variables}}`) que ya renderiza el worker.

| | `builder` | `html` |
|---|---|---|
| Se edita con | Editor de bloques (actual) | Editor de código + preview |
| Se guarda | `structure` (bloques JSON) | `body` (HTML/CSS crudo, ya saneado) |
| `body` | compilado desde `structure` | = el HTML del usuario (saneado) |
| Fidelidad | limitada a los bloques | total |
| Cambio de modo | `builder → html` (compila y editas el código, ida sola) | "volver a visual" **descarta** el HTML y recupera los bloques; `html → builder` real **no** |

Regla clave: en `html`, `body` guarda el HTML final; **el render de envío no necesita
rama** (`processor.go:356` ya hace `template.Render(tmpl.Body, payload)`). Solo cambian
guardado, preview, extracción de variables y el editor.

## 2. Modelo de datos

Migración nueva `db/migrations/00007_html_mode.sql`:

- `templates`:
  - `ADD COLUMN kind text NOT NULL DEFAULT 'builder' CHECK (kind IN ('builder','html'))`.
  - `structure` sigue `NOT NULL`. En `html` **se conserva el último `structure` de bloques**
    (si viene de un builder) para permitir "volver a visual"; si es `html` nativa se guarda `'{}'::jsonb`.
  - `body` ya existe (`NOT NULL`): en `html` guarda el HTML saneado.
- `base_templates` (hoy no tiene `body` ni `subject`):
  - `ADD COLUMN kind text NOT NULL DEFAULT 'builder' CHECK (...)`.
  - `ADD COLUMN body text` (nullable) → HTML crudo de las bases `html`.
  - `ADD COLUMN subject text` (nullable) → para sugerir asunto al clonar (las 12 traen buenos asuntos que hoy se perderían).

Sin cambios destructivos: todas las filas existentes quedan `kind='builder'` y siguen igual.

Regenerar sqlc tras la migración (`make sqlc`) — afecta a `internal/store/sqlc`.

## 3. Backend (Go)

### 3.1 sqlc / queries (`db/queries/`)
- `templates.sql` → `CreateTemplateFull` (línea 25): añadir `kind` a columnas y params.
- `base_templates.sql` → `ListBaseTemplates`/`GetBaseTemplate*`: incluir `kind`, `body`, `subject`.

### 3.2 Servicio (`internal/template/service.go`)
- `Input` (create/update): añadir `Kind string` y `Body string` (HTML crudo, opcional).
- `persistVersion()` (líneas 109-161): **bifurcar por kind**
  - `builder`: flujo actual (`CompileForChannel` → `DeriveVariables` → `BuildRequiredSchema`).
  - `html`:
    1. `body := sanitizeEmailHTML(in.Body)` (§3.4).
    2. `vars := DeriveVariablesFromText(in.Subject + " " + in.Body)` (§3.3).
    3. `schema := BuildRequiredSchema(vars, in.Variables)`.
    4. Guardar `body`, `structure` = los bloques conservados (o `'{}'` si es html nativa),
       `required_variables=schema`, `kind='html'`. Conservar `structure` es lo que permite "volver a visual".
- `PreviewDraft()` (líneas 270-293): bifurcar igual; en `html` sanear el HTML entrante, renderizar `subject`+`body` con el payload y devolver `PreviewResult`.
- `resolveStructure`/clonado desde base: si la base es `html`, copiar `body`+`subject`+`kind`.

### 3.3 Extracción de variables desde HTML (`internal/template/builder.go`)
- Reutilizar la regex de `DeriveVariables` (líneas 345/362-398) en una función nueva
  `DeriveVariablesFromText(s string) []string` que escanea `{{var}}` en un string plano
  (no en `structure`). Misma exclusión de keywords Handlebars (`if/each/...`).

### 3.4 Saneamiento (el punto crítico) — nuevo `internal/template/sanitize.go`
Los correos son **documentos completos con `<style>`**, así que un saneador de
fragmentos (p. ej. bluemonday tal cual) **no sirve**: elimina `<head>`/`<style>`.
Enfoque correcto: recorrer el árbol con `golang.org/x/net/html` y aplicar allowlist.

- **Elimina** (defensa): `<script>`, `<iframe>`, `<object>`, `<embed>`, `<link>`,
  `<base>`, `<form>`, `<meta http-equiv=refresh>`; atributos `on*` (onclick, onerror…);
  URLs con esquema `javascript:`/`vbscript:`/`data:` (salvo `data:image/*`) en `href`/`src`.
- **Conserva** (email): estructura del documento, `<style>`, `style=` inline, tablas,
  `img`, `a`, listas, encabezados, etc.
- **CSS** (dentro de `<style>` y en `style=`): quitar `expression(...)`, `@import`,
  `url(javascript:...)`, `behavior:`, `-moz-binding`.
- Límite de tamaño del `body` (p. ej. 256 KB) → error de validación si se excede.
- Se aplica **al guardar y al previsualizar** (nunca confiar en el cliente).
- Nota: raymond auto-escapa `{{x}}` en render; **mantener doble llave** (no `{{{x}}}`),
  así los valores del payload van escapados en contexto HTML.

### 3.5 Handlers admin (`internal/http/admin_template_handlers.go`)
- `templateRequest` (líneas 27-37): añadir `Kind`, `Body`.
- `previewDraftRequest` (líneas 213-228): añadir `Kind`, `Body`.
- `templateView` (254-267) y `baseTemplateView`: exponer `Kind`, y en bases `Body`/`Subject`.
- Validación: si `kind='html'`, `Body` obligatorio y `Structure` ignorada (y viceversa).

### 3.6 Envío (`internal/notification/processor.go`)
- **Sin cambios** en el render (`Render(tmpl.Body, payload)` ya cubre ambos modos).
- La validación de payload sigue usando `required_variables` (que ya generamos en `html`).

## 4. Seed de las 12 en modo HTML (`cmd/seed`)
- Incorporar las 12 del CSV como **bases `html`** (fidelidad total), además de las bases
  `builder` ya sembradas.
- Guardar los HTML en el repo y cargarlos con `embed`: `cmd/seed/htmlbases/*.html`
  (+ un índice JSON con `key/name/category/subject/suggested`). Nada de leer archivos
  externos en runtime.
- **Sanear en la importación** con el mismo `sanitizeEmailHTML`.
- `INSERT ... ON CONFLICT (key) DO UPDATE` (ya lo dejamos así) para refrescar.
- Convención de claves para no chocar con las `builder`: sufijo `-html`
  (p. ej. `welcome-html`, `otp-html`) **o** una columna que las distinga en el listado.
  Recomendado: sufijo `-html` + `kind='html'`, así conviven las dos versiones de cada una.

## 5. Frontend (SPA)

### 5.1 Editor de código
- Añadir dependencias: `@uiw/react-codemirror` + `@codemirror/lang-html` (ligero, Vite-friendly).
- Nuevo componente `web/src/pages/tabs/HtmlBuilder.tsx` (hermano de `EmailBuilder.tsx`):
  - **Izquierda**: editor de código (HTML con resaltado). Un único editor con el
    documento completo (el CSS va en su `<style>`); **no** un campo CSS aparte.
  - **Derecha**: preview en vivo (mismo `POST .../templates/preview`, ahora con `{kind:'html', body}`)
    + `VariablesPanel` + `SendObjectCard`. Reutiliza `testData`/`detectVariables`
    (detectando `{{...}}` sobre el string HTML).

### 5.2 Selector de modo (`web/src/pages/TemplateEditor.tsx`)
- Estado `editorMode: 'visual' | 'html'` (solo `channel === 'EMAIL'`).
- Conmutador con shadcn `Tabs` cerca del bloque "Partir de una base" (~línea 373), oculto al clonar.
- Render condicional: `{editorMode === 'visual' ? <EmailBuilder/> : <HtmlBuilder/>}`.
- `save()` (líneas 262-291): si `html`, enviar `{kind:'html', body, structure}` (el `structure`
  conservado permite revertir); si `visual`, como hoy.
- **Pasar a HTML**: al conmutar `visual → html`, precargar el editor con el `body` compilado
  (último preview) y **conservar `form.structure`** en estado.
- **Volver a visual**: botón que descarta el HTML y restaura el editor de bloques desde
  `form.structure`; con confirmación ("se perderán los cambios de HTML"). Solo visible si hay `structure` de bloques.
- Al clonar una base `html`, fijar `editorMode='html'` y precargar `body`+`subject`.

### 5.3 Sandbox del preview (seguridad — obligatorio)
- Añadir `sandbox=""` a los tres iframes de preview:
  `TemplateEditor.tsx:544`, `EmailBuilder.tsx:427` y el nuevo de `HtmlBuilder`.
  Con `sandbox=""` el HTML se pinta pero **no ejecuta scripts** en el navegador del admin.

### 5.4 Base templates
- El endpoint `GET /admin/base-templates` debe devolver `kind` (+ `body`/`subject` en `html`)
  para que el selector precargue el modo correcto.

## 6. Seguridad (checklist)
1. **Sanear en servidor** al guardar y al previsualizar (§3.4). Nunca confiar en el cliente.
2. **Escapar variables** en render (raymond por defecto; mantener `{{x}}`).
3. **Sandbox** en los iframes de preview (`sandbox=""`).
4. **Autorización**: modo `html` disponible para **todo admin** (decidido). Sin gate por rol;
   se apoya en la auditoría (punto 5) para trazar quién edita.
5. **Auditoría**: registrar creación/edición de plantillas `html` en `admin_audit_logs`.
6. **Límite de tamaño** del `body`.
7. Los `<img>/<a>` a URLs externas son normales en email (el servidor no las descarga → sin SSRF).

## 7. Fases y entregables (commits independientes)

| Fase | Contenido | Entregable verificable |
|---|---|---|
| **1. Datos + backend** | Migración 00007, sqlc, `sanitize.go`, `DeriveVariablesFromText`, rama en `persistVersion`/`PreviewDraft`, handlers | Crear/guardar/enviar una plantilla `html` vía API; preview `html` funciona |
| **2. Seed 12 HTML** | `embed` de los 12 HTML + import saneado como bases `html` | `base_templates` con las 12 `html`; visibles en `GET /admin/base-templates` |
| **3. SPA** | `HtmlBuilder`, selector de modo, sandbox de iframes, clonado de base `html` | Editar HTML con preview en vivo; crear desde base `html` |
| **4. Hardening** | Gate por rol, auditoría, límite de tamaño, tests, docs | Suite verde + doc actualizada |

## 8. Pruebas (sin bugs)
- **Go unit** (`internal/template`):
  - `sanitize_test.go`: quita `<script>`/`on*`/`javascript:`/`<iframe>`; conserva `<style>`, `style=`, tablas, `img`; neutraliza `expression()`/`@import`.
  - `DeriveVariablesFromText`: detecta y deduplica variables; ignora keywords Handlebars.
  - `service_test.go`: `persistVersion` en `html` guarda body saneado + schema; `PreviewDraft` html.
  - Regresión: los tests actuales del builder siguen verdes (ruta `builder` intacta).
- **Frontend** (vitest): detección de variables desde HTML; conmutación de modo.
- **Manual**: la página de preview y el editor en `localhost:5173`.

## 9. Riesgos (decisiones ya cerradas en §0)
- **Saneador de email con `<style>`** (riesgo principal): saneador propio con `x/net/html`
  (no bluemonday a secas) y batería de tests dedicada. Es lo que más cuidado y cobertura lleva.
- **Conservar `structure` en modo html**: hay que asegurarse de no "ensuciarlo" al pasar a HTML,
  para que "volver a visual" restaure exactamente el último estado de bloques.
- **Preview sin sandbox hoy**: no olvidar añadir `sandbox=""` a los 3 iframes (regresión de seguridad si se omite).

## 10. Estimación (orden de magnitud)
- Fase 1: la mayor (saneador + tests). Fase 2: media (contenido). Fase 3: media (editor + UX).
  Fase 4: pequeña. Cada fase cierra con su commit y sus tests.

## 11. Estado — ✅ COMPLETADO
- **Fase 1–3**: hechas (backend+saneador, 12 bases html, SPA con editor de código).
- **Fase 4 (hardening)**: cerrada. Saneado (guardar+preview), escape de variables
  (+ fix del bypass triple-stache), sandbox de iframes, endurecido (svg/math/comentarios/
  srcset), límite de tamaño 256 KB, auditoría (mismo `audit.Record` de plantillas),
  gate por rol N/A (para todo admin), tests, y **docs en `CLAUDE.md`** (§ "Template modes").
