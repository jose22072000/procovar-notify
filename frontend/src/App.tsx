import { Routes, Route, Navigate, Link } from "react-router-dom";
import { lazy, Suspense, type ReactNode } from "react";
import { useAuth } from "./auth/auth";
import Login from "./pages/Login";
import Applications from "./pages/Applications";
import AppDetail from "./pages/AppDetail";
import AdminUsers from "./pages/AdminUsers";
import UserMenu from "./components/UserMenu";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { ThemeToggle } from "./theme";

// El editor de plantillas carga CodeMirror + js-beautify + dnd-kit; se carga en
// diferido (lazy) para no engordar el bundle inicial de la SPA.
const TemplateEditor = lazy(() => import("./pages/TemplateEditor"));
const EditorRoute = () => (
  <Suspense fallback={<div className="p-6 text-sm text-muted-foreground">Cargando editor…</div>}>
    <TemplateEditor />
  </Suspense>
);

function Protected({ children }: { children: ReactNode }) {
  const { admin, comprobando } = useAuth();
  // Mientras se comprueba si ya hay sesión de Procovar NO se pinta nada.
  //
  // Antes se navegaba a /login de inmediato, así que se veía la pantalla de
  // Avisos un instante antes de saltar al hub. Enseñar y quitar es peor que no
  // enseñar: parece que la aplicación se equivocó y se corrigió sola.
  if (comprobando) return null;
  return admin ? <>{children}</> : <Navigate to="/login" replace />;
}

function Shell({ children }: { children: ReactNode }) {
  return (
    <div className="min-h-screen">
      <header className="border-b border-border bg-card">
        <div className="mx-auto flex max-w-[1440px] items-center justify-between px-4 py-3 sm:px-6">
          <Link to="/apps" className="flex items-center gap-2 transition-opacity hover:opacity-80" title="Inicio">
            <span className="flex h-7 w-7 items-center justify-center rounded-lg bg-primary text-primary-foreground">
              <svg viewBox="0 0 24 24" fill="none" className="h-4 w-4" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9" />
                <path d="M13.73 21a2 2 0 0 1-3.46 0" />
              </svg>
            </span>
            <span className="text-lg font-semibold tracking-tight text-foreground">QBNotify</span>
          </Link>
          <div className="flex items-center gap-1">
            <ThemeToggle />
            <UserMenu />
          </div>
        </div>
      </header>
      <main className="mx-auto max-w-[1440px] px-4 py-5 sm:px-6 sm:py-6">
        {/* Contiene el fallo de una página sin perder la cabecera/navegación. */}
        <ErrorBoundary>{children}</ErrorBoundary>
      </main>
    </div>
  );
}

export default function App() {
  const { admin, comprobando } = useAuth();
  // Nada en pantalla hasta saber si ya hay sesión: ni el panel ni el login.
  // Es lo que evita el parpadeo de "Avisos" antes de saltar al hub.
  if (comprobando) return null;
  return (
    <Routes>
      <Route path="/login" element={admin ? <Navigate to="/apps" replace /> : <Login />} />
      <Route
        path="/apps"
        element={
          <Protected>
            <Shell>
              <Applications />
            </Shell>
          </Protected>
        }
      />
      <Route
        path="/apps/:appId"
        element={
          <Protected>
            <Shell>
              <AppDetail />
            </Shell>
          </Protected>
        }
      />
      <Route
        path="/apps/:appId/templates/new"
        element={
          <Protected>
            <Shell>
              <EditorRoute />
            </Shell>
          </Protected>
        }
      />
      <Route
        path="/apps/:appId/templates/:templateId/edit"
        element={
          <Protected>
            <Shell>
              <EditorRoute />
            </Shell>
          </Protected>
        }
      />
      <Route
        path="/admins"
        element={
          <Protected>
            <Shell>
              <AdminUsers />
            </Shell>
          </Protected>
        }
      />
      <Route path="*" element={<Navigate to="/apps" replace />} />
    </Routes>
  );
}
