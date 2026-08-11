# QB Notify — SPA de administración

Panel de administración (Vite + React + TypeScript + Tailwind) que consume la API
`/admin` del servicio Go.

## Desarrollo

```bash
npm install
npm run dev      # http://localhost:5173 (proxy a la API en :8080)
```

Arranca antes la API (`make run-api`) y la infra (`make up`). Login con el
super-admin sembrado (`admin@qbnotify.local` / `changeme-admin` por defecto).

## Build

```bash
npm run build    # genera dist/ (estático para CDN/Nginx)
npm run preview
```

## Estructura

- `src/api/client.ts` — cliente HTTP (JWT + refresh + problem+json).
- `src/auth/` — contexto de autenticación.
- `src/pages/` — Login, Aplicaciones y detalle de aplicación con pestañas:
  API Keys, SMTP, Proveedores, Rutas, Templates (+preview), Notificaciones
  (+detalle/retry/cancel), Monitor de cola (SSE en vivo) y Métricas.

El editor de templates usa secciones en JSON con preview; el builder visual
drag-and-drop queda como evolución futura.
