// Cliente HTTP de la API de administración: añade el access token (Bearer),
// renueva ante un 401 con el refresh token —que vive en una cookie HttpOnly, no
// en JS— y normaliza los errores application/problem+json.

const ACCESS_KEY = "qbn_access";

// Base del API: vacía en dev (usa el proxy de Vite, same-origin) y en
// producción cuando hay un reverse proxy delante; con VITE_API_URL apunta
// directo al dominio público del API (build-time, horneado en el bundle).
export const API_BASE = import.meta.env.VITE_API_URL ?? "";

// El refresh token viaja en una cookie HttpOnly gestionada por el navegador; el
// cliente solo maneja el access token (corto, 15 min). credentials:"include" en
// cada petición envía esa cookie también cuando el API está en otro dominio.
export const tokens = {
  access: () => localStorage.getItem(ACCESS_KEY),
  set(access: string) {
    localStorage.setItem(ACCESS_KEY, access);
  },
  clear() {
    localStorage.removeItem(ACCESS_KEY);
  },
};

export class ApiError extends Error {
  status: number;
  code: string;
  constructor(status: number, code: string, detail: string) {
    super(detail || code);
    this.status = status;
    this.code = code;
  }
}

async function rawFetch(method: string, path: string, body?: unknown): Promise<Response> {
  const headers: Record<string, string> = {};
  const access = tokens.access();
  if (access) headers["Authorization"] = `Bearer ${access}`;
  if (body !== undefined) headers["Content-Type"] = "application/json";
  return fetch(`${API_BASE}${path}`, {
    method,
    headers,
    credentials: "include", // envía la cookie HttpOnly del refresh (también cross-domain)
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
}

// refreshInFlight deduplica renovaciones concurrentes: si varias peticiones
// reciben 401 a la vez, todas esperan al MISMO refresh en vuelo en lugar de
// canjear el refresh token N veces en paralelo (llamadas redundantes y un
// last-writer-wins innecesario sobre localStorage). Se limpia al terminar.
let refreshInFlight: Promise<boolean> | null = null;

export function tryRefresh(): Promise<boolean> {
  if (refreshInFlight) return refreshInFlight;
  refreshInFlight = doRefresh().finally(() => {
    refreshInFlight = null;
  });
  return refreshInFlight;
}

async function doRefresh(): Promise<boolean> {
  // El refresh token va en la cookie HttpOnly: no se manda cuerpo, el navegador
  // adjunta la cookie (credentials:"include"). El servidor rota la cookie y
  // devuelve un access nuevo.
  const res = await fetch(`${API_BASE}/admin/auth/refresh`, {
    method: "POST",
    credentials: "include",
  });
  if (!res.ok) return false;
  // Un cuerpo no-JSON o sin access (proxy, respuesta corrupta) no debe lanzar un
  // SyntaxError crudo ni guardar "undefined": se trata como refresh fallido y
  // api() disparará onUnauthorized.
  let data: { accessToken?: string } | null = null;
  try {
    data = await res.json();
  } catch {
    return false;
  }
  if (!data?.accessToken) return false;
  tokens.set(data.accessToken);
  return true;
}

async function parse<T>(res: Response): Promise<T> {
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  // Un cuerpo no-JSON (p. ej. un 5xx con HTML de un proxy) no debe lanzar un
  // SyntaxError crudo: se trata como sin datos y se normaliza a ApiError.
  let data: { code?: string; detail?: string } | null = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = null;
    }
  }
  if (!res.ok) {
    const code = data?.code ?? "error";
    const detail = data?.detail ?? res.statusText;
    throw new ApiError(res.status, code, detail);
  }
  return data as T;
}

// onUnauthorized se invoca cuando una petición autenticada acaba en 401 y el
// refresh no pudo renovar la sesión (sesión muerta). El AuthProvider lo registra
// para cerrar sesión y mandar al login, en vez de dejar la vista en blanco con el
// error "Invalid or expired token".
let onUnauthorized: (() => void) | null = null;
export function setOnUnauthorized(fn: (() => void) | null) {
  onUnauthorized = fn;
}

export async function api<T>(method: string, path: string, body?: unknown): Promise<T> {
  let res = await rawFetch(method, path, body);
  const isAuthPath = path.startsWith("/admin/auth");
  // Renovación transparente ante 401 (salvo en los propios endpoints de auth).
  if (res.status === 401 && !isAuthPath) {
    if (await tryRefresh()) {
      res = await rawFetch(method, path, body);
    }
    // Si tras intentar refrescar seguimos en 401, la sesión está muerta.
    if (res.status === 401) {
      onUnauthorized?.();
    }
  }
  return parse<T>(res);
}

export const get = <T>(path: string) => api<T>("GET", path);
export const post = <T>(path: string, body?: unknown) => api<T>("POST", path, body);
export const put = <T>(path: string, body?: unknown) => api<T>("PUT", path, body);
export const patch = <T>(path: string, body?: unknown) => api<T>("PATCH", path, body);
export const del = <T>(path: string) => api<T>("DELETE", path);
