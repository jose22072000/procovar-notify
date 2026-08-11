import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { post, setOnUnauthorized, tokens } from "../api/client";

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
  }

  function logout() {
    // Avisa al servidor para revocar el refresh (incrementa token_version) y
    // borrar la cookie; el fallo de red no debe impedir cerrar sesión localmente.
    // Promise.resolve envuelve el fire-and-forget para no lanzar si post no
    // devolviera una promesa.
    void Promise.resolve(post("/admin/auth/logout")).catch(() => {});
    tokens.clear();
    localStorage.removeItem(ADMIN_KEY);
    setAdmin(null);
  }

  return <AuthContext.Provider value={{ admin, login, logout }}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth fuera de AuthProvider");
  return ctx;
}
