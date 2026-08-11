# Plan de cobertura de tests — red de seguridad para refactors

> Objetivo: que un cambio que rompa comportamiento **falle un test**, no que llegue
> a producción. Foco en lo más frágil/reciente (el editor de plantillas) y en cerrar
> los dos huecos reales: **frontend sin tests** y **CI sin frontend**.

## Línea base (estado actual)

- **Backend Go**: 23/26 paquetes `internal/` con tests, muchos de **integración**
  (testcontainers/Postgres). Sólido en lo fundamental (auth, envío, plantillas+saneador,
  cuotas, webhooks, canales, retención). Sin tests justificado: `store/sqlc` (generado),
  `storetest` (harness), `metrics`. `cmd/*` sin unit tests.
- **Frontend**: solo 5 archivos de test (`ui`, `theme`, `auth`, `api/client`,
  `templateBuilder`). El **editor** (`TemplateEditor`, `HtmlBuilder`, `VariablesPanel`,
  `EmailBuilder`) sin tests. Tooling ya instalado: vitest + jsdom + @testing-library.
- **CI** (`.github/workflows/ci.yml`): Go (vet, golangci-lint, `go test -race`, build,
  smoke). **No corre el frontend** (ni `npm test`, ni `tsc`, ni build). Dispara solo en
  `version-2`/`main` (no en ramas `feature/*`).
- `docker-build.yml` construye el `Dockerfile` de la raíz = **v1 legado** (a revisar).

## Fase 1 — Frontend: lógica pura del editor (rápido, alto valor) ✅

Extraer las funciones puras del editor a un módulo testeable y cubrirlas con vitest
(sin render). Son las que más se tocan en refactors:
- Nuevo `web/src/pages/templateEditor.utils.ts` con: `formatHtml`, `structureHasBlocks`,
  `sampleValue`, `toStructure`, `specsFromSchema`, `defaultStructure` (hoy embebidas en
  el componente).
- Ampliar `templateBuilder.test.ts`: `detectVariablesInText`, `uid`, `blocksOf`,
  `fontImportFor`, casos de `detectVariables` con `{{#if}}`/`{{#each}}`.
- Fija el contrato de: detección de variables, formateo, heurística de valores de prueba,
  normalización de estructura.

## Fase 2 — Frontend: comportamiento del editor (lo que ya se rompió) ✅

Tests de componente (@testing-library/react + mock del `api/client`):
- **`save()` envía el `kind` correcto** en cada combinación (nuevo/editar × visual/html)
  → bloquea el bug de revert html→visual que arreglamos.
- **`switchMode`**: visual→html siembra el body compilado; html→visual abre el modal y
  al confirmar restaura los bloques.
- **Filtro de bases por modo** y clonado de base html (precarga body+subject).
- **Preview**: payload correcto según modo + guard de carrera (respuesta obsoleta ignorada).
- `VariablesPanel`: editar tipo/obligatoria/valor/descripción dispara callbacks.
- `Templates`: badge de kind y filtro por canal.

## Fase 3 — Backend: cerrar huecos y reforzar lo fino ✅ (parcial)

Foco en el hueco mayor y más relevante: los **handlers de plantillas** (nuestra feature).
- `admin_template_handlers_test.go`: validación sin BD (missing_fields, invalid_json,
  invalid_base_template_id, invalid_app_id) + `PreviewDraft` html/builder con servicio
  real (db nil) que ejercita saneado + render a nivel HTTP.
- `admin_template_handlers_integration_test.go`: CRUD completo con BD (testcontainers)
  — Create (html, guardado saneado), Get, List, Delete.
- Resultado: `internal/http` 5.2% → **10.9%**; los handlers de plantillas cubiertos.
- **Pendiente (ratchet futuro):** el resto de handlers de `internal/http` (api-keys,
  smtp, providers, routes, monitor, audit, webhooks, suppressions, recurring, quota…)
  necesitan cada uno su test de integración; y subir `store`/`admin`/`recurring`.

## Fase 4 — Prevención de regresiones (CI + gates) — la clave ✅

- **Job de frontend en `ci.yml`** ✅: `npm ci`, `tsc --noEmit`, `npm run test -- --coverage`,
  `npm run build` (gate must-pass del frontend, que antes NO se ejecutaba en CI).
- **Triggers ampliados** ✅: push en `version-2`/`main`/`feature/**` y **cualquier PR**.
- **Coverage**:
  - Frontend ✅: `@vitest/coverage-v8` + **umbral ratchet** en `vite.config.ts`
    (statements ≥11, branches ≥70, functions ≥45) — suelo anti-regresión, subir al
    añadir tests. Gate duro (falla si baja).
  - Go: reporte de cobertura en CI (`-coverprofile` + resumen `go tool cover -func`).
    Sin umbral duro aún (se fijará desde el primer número real del CI, para no romper
    por un floor mal estimado).
- **`docker-build.yml`** ✅: pasaba a publicar la imagen v1 en cada push a `main`; ahora
  es **manual** (`workflow_dispatch`) hasta que exista un Dockerfile del v2.

## Fase 5 — E2E ligero (opcional) ✅

- `internal/notification/html_e2e_test.go`: flujo completo de una plantilla en MODO
  HTML — crear con `template.Service` (el HTML se **sanea**) → SMTP + ruta →
  `notification.Service.Create` → `Processor.ProcessSend` → un sender captura el
  mensaje y se verifica que el HTML enviado va **saneado** (sin `<script>`) y
  **renderizado con el payload** (`{{firstName}}` → "Jane"). Con BD (testcontainers),
  sin depender de Redis/SMTP reales → corre en CI.
- `deploy/smoke.sh` se mantiene como smoke de conectividad (`/readyz`) en un job aparte.

## Convención (para no volver a acumular deuda)

Cada PR que toque lógica trae su test: UI con @testing-library, lógica pura con vitest,
backend con testcontainers. El CI (Fase 4) lo hace exigible.

## Orden sugerido
Fase 1 → 2 (protege lo más frágil), luego **Fase 4** (para que a partir de ahí nada entre
sin tests), y 3/5 en paralelo/después.
