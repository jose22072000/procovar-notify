# Plan de correcciones — hallazgos de la revisión

> Los hallazgos **críticos/altos ya se corrigieron** (commits `5507dc1`, `625cad5`:
> bypass triple-stache, revert html→visual, endurecimiento del saneador, carrera
> del preview, temp file, METRICS_PORT). Este plan cubre lo **restante**, por fases,
> de más a menos importante. Rama: `feature/cambios-ui-v2`. No se hace push hasta indicarlo.

## Fase 1 — Errores silenciosos y robustez (importante) ✅

Fallos que hoy se tragan sin señal: pueden ocultar problemas reales.

1. **Escrituras de auditoría ignoradas** — `_ = ...CreateNotificationLog(...)` en
   `internal/notification/{suppression.go,query.go,processor.go,service.go}` y
   `internal/monitor/service.go`. `NotificationLog` es el rastro de auditoría
   append-only; perderlo sin log crea huecos invisibles. → `logger.Warn` en el fallo.
2. **`json.Unmarshal` de `payload`/`recipient` ignorado** — `internal/notification/processor.go`
   y `internal/recurring/service.go`: JSON malformado ⇒ struct vacío en silencio.
   → comprobar y loguear el error.
3. **TOCTOU de la versión de plantilla** — `GetMaxTemplateVersion` se lee fuera de la
   `Tx`; dos ediciones concurrentes calculan la misma versión y una da un `409`
   confuso. → calcular `max+1` dentro de la transacción.

## Fase 2 — Código muerto y estructura (secundario) ✅

4. **Docker de la raíz construye el v1** — `/Dockerfile` y `/docker-compose.yml` son
   el stack legado (Node/Prisma). → banner de aviso "LEGADO v1" al principio de ambos,
   apuntando a `deploy/` y al Makefile del v2 (no destructivo; se conservan como referencia).
5. **`retention.ForgetUser` muerto** — no lo usaba ni producción ni su test (el real
   vive en `internal/admin/recurring.go`). → eliminado (+ imports sobrantes).

## Fase 3 — DRY / calidad de código (secundario) ✅

6. **Interfaz `Preview` declarada 3 veces** → un tipo `Preview` en `TemplateBuilder.tsx`.
7. **`detectFromText` duplica `detectVariables`** → `detectVariablesInText` compartido.
8. **`uid()` definido dos veces** → `uid()` compartido en `TemplateBuilder.tsx`.

## Fase 4 — UI / accesibilidad / rendimiento (terciario) ✅

10. **Avisos duplicados** ("Faltan variables" + "vista previa en pausa") → componente
    compartido `PreviewNotices.tsx` (`MissingVarsNotice`/`PreviewPausedNotice`); el
    color amber se centraliza en un solo sitio (el tema no tiene token "warning").
11. **Bundle único de 1.24 MB** → **lazy-load** de la ruta del editor: el bundle
    inicial baja a **432 KB** y CodeMirror/js-beautify/dnd-kit van en un chunk aparte.
9. **Tabs sin `TabsContent`** (a11y) — *aceptado sin cambio*: Radix `Tabs` como
   segmented control funciona con teclado; el `aria-controls` colgando es un aviso
   menor de validador y convertirlo arriesga regresiones de estilo por valor bajo.
12. **Saneador CSS por regex** (`expr/**/ession`) — *aceptado/documentado*: solo IE,
   impacto ínfimo; ya se bloquean `svg`/`math` y `expression`/`@import`/`behavior`.

## Estado
- Fases 1–4: ✅ hechas (9 y 12 aceptadas con rationale; el resto corregido).
