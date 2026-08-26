import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { get, post, setOnUnauthorized, tokens } from "../api/client";

export interface Admin {
  id: string;
  role: string;
  applicationId?: string;
  email?: string;
}

interface LoginResponse {
  accessToken: string;
  admin: Admin;
}

interface AuthState {
  admin: Admin | null;
  /** true mientras se comprueba si ya hay sesión de Procovar (SSO). */
  comprobando: boolean;
  login: (email: string, password: string) => Promise<void>;
  logout: () => void;
}

const ADMIN_KEY = "qbn_admin";
const AuthContext = createContext<AuthState | null>(null);

function loadAdmin(): Admin | null {
  // Defensivo: un valor corrupto/de un formato antiguo (o localStorage no
  // disponible) no debe tumbar la app — se descarta y se trata como no logueado.
  try {
    const raw = localStorage.getItem(ADMIN_KEY);
    return raw ? (JSON.parse(raw) as Admin) : null;
  } catch {
    try {
      localStorage.removeItem(ADMIN_KEY);
    } catch {
      /* localStorage no disponible */
    }
    return null;
  }
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [admin, setAdmin] = useState<Admin | null>(loadAdmin());
  // Solo se comprueba si NO había sesión guardada: quien ya entró no espera.
  const [comprobando, setComprobando] = useState(() => loadAdmin() === null);

  // Sesión única de Procovar.
  //
  // Sin esto el SSO no sirve de nada de cara al usuario: el backend acepta la
  // cookie de procovar-auth, pero la SPA mira `localStorage`, no encuentra
  // nada y manda al formulario de login igualmente. La cookie viaja sola
  // —`credentials:"include"` en cada petición y el dominio es
  // `.procovar.cloud`—, así que basta con PREGUNTAR una vez.
  //
  // Un 401 aquí es lo normal para quien no ha entrado en Procovar: se traga en
  // silencio y se enseña el login de siempre.
  useEffect(() => {
    if (admin !== null) return;
    let vivo = true;
    void (async () => {
      try {
        const yo = await get<Admin>("/admin/me");
        if (!vivo || !yo?.id) return;
        localStorage.setItem(ADMIN_KEY, JSON.stringify(yo));
        setAdmin(yo);
      } catch {
        /* sin sesión de Procovar: al login normal */
      } finally {
        if (vivo) setComprobando(false);
      }
    })();
    return () => {
      vivo = false;
    };
    // Solo al montar: si `admin` cambia es porque ya se entró.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Si una petición autenticada acaba en 401 y el refresh no renueva (sesión
  // caducada/inválida), el cliente avisa aquí: limpiamos la sesión para que la
  // guardia redirija al login en vez de dejar una vista en blanco.
  useEffect(() => {
    setOnUnauthorized(() => {
      tokens.clear();
      try {
        localStorage.removeItem(ADMIN_KEY);
      } catch {
        /* localStorage no disponible */
      }
      setAdmin(null);
    });
    return () => setOnUnauthorized(null);
  }, []);

  async function login(email: string, password: string) {
    const res = await post<LoginResponse>("/admin/auth/login", { email, password });
    // El refresh token lo fija el servidor como cookie HttpOnly; aquí solo el access.
    tokens.set(res.accessToken);
    // El backend no devuelve el email; lo conservamos del formulario para la UI.
    const withEmail: Admin = { ...res.admin, email };
    localStorage.setItem(ADMIN_KEY, JSON.stringify(withEmail));
    setAdmin(withEmail);
    setComprobando(false);
  }

  function logout() {
    // Se borra la marca de "ya fui al hub" (index.html) para que la siguiente
    // visita vuelva a saltar sola. Sin esto, quien cierra sesión se queda en
    // esta pestaña viendo la pantalla de Avisos en vez de ir a Procovar.
    try {
      sessionStorage.removeItem("sso_ida");
    } catch {
      /* sin almacenamiento */
    }
    // Avisa al servidor para revocar el refresh (incrementa token_version) y
    // borrar la cookie; el fallo de red no debe impedir cerrar sesión localmente.
    // Promise.resolve envuelve el fire-and-forget para no lanzar si post no
    // devolviera una promesa.
    void Promise.resolve(post("/admin/auth/logout")).catch(() => {});
    tokens.clear();
    localStorage.removeItem(ADMIN_KEY);
    setAdmin(null);
  }

  return <AuthContext.Provider value={{ admin, comprobando, login, logout }}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth fuera de AuthProvider");
  return ctx;
}
