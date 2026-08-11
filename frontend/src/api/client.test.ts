import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { get, post, del, ApiError, setOnUnauthorized } from "./client";

// res construye una respuesta mínima compatible con lo que usa el cliente
// (status, ok, text(), json()).
function res(status: number, body?: unknown): Response {
  return {
    status,
    ok: status >= 200 && status < 300,
    statusText: "",
    text: async () => (body === undefined ? "" : JSON.stringify(body)),
    json: async () => body,
  } as unknown as Response;
}

let fetchMock: ReturnType<typeof vi.fn>;

function authHeader(call: unknown[]): string | undefined {
  return (call[1] as { headers: Record<string, string> }).headers.Authorization;
}

beforeEach(() => {
  localStorage.clear();
  fetchMock = vi.fn();
  vi.stubGlobal("fetch", fetchMock);
});
afterEach(() => vi.unstubAllGlobals());

describe("api client", () => {
  it("añade el Bearer y devuelve el JSON en un GET correcto", async () => {
    localStorage.setItem("qbn_access", "tok");
    fetchMock.mockResolvedValueOnce(res(200, { ok: true }));

    const out = await get<{ ok: boolean }>("/admin/me");
    expect(out).toEqual({ ok: true });
    expect(fetchMock.mock.calls[0][0]).toBe("/admin/me");
    expect(authHeader(fetchMock.mock.calls[0])).toBe("Bearer tok");
  });

  it("ante 401 renueva (cookie) y reintenta con el nuevo access", async () => {
    localStorage.setItem("qbn_access", "viejo");
    fetchMock
      .mockResolvedValueOnce(res(401, { code: "token_expired" })) // original
      .mockResolvedValueOnce(res(200, { accessToken: "nuevo" })) // refresh (cookie HttpOnly)
      .mockResolvedValueOnce(res(200, { data: [1, 2] })); // reintento

    const out = await get<{ data: number[] }>("/admin/applications");

    expect(out).toEqual({ data: [1, 2] });
    expect(fetchMock).toHaveBeenCalledTimes(3);
    // El refresh pega al endpoint correcto y el reintento usa el nuevo access.
    expect(fetchMock.mock.calls[1][0]).toBe("/admin/auth/refresh");
    expect(localStorage.getItem("qbn_access")).toBe("nuevo");
    expect(authHeader(fetchMock.mock.calls[2])).toBe("Bearer nuevo");
  });

  it("el refresh no manda el token en el cuerpo (viaja en la cookie HttpOnly)", async () => {
    localStorage.setItem("qbn_access", "viejo");
    fetchMock
      .mockResolvedValueOnce(res(401, { code: "token_expired" }))
      .mockResolvedValueOnce(res(200, { accessToken: "nuevo" }))
      .mockResolvedValueOnce(res(200, { ok: true }));

    await get("/admin/x");

    const refreshCall = fetchMock.mock.calls[1];
    expect(refreshCall[0]).toBe("/admin/auth/refresh");
    // Sin cuerpo (el refresh token va en la cookie) y con credenciales incluidas.
    expect((refreshCall[1] as RequestInit).body).toBeUndefined();
    expect((refreshCall[1] as RequestInit).credentials).toBe("include");
  });

  it("si el refresh falla, propaga el 401 sin reintentar la original", async () => {
    fetchMock
      .mockResolvedValueOnce(res(401, { code: "token_expired" }))
      .mockResolvedValueOnce(res(401)); // el refresh (cookie) falla
    await expect(get("/admin/x")).rejects.toMatchObject({ status: 401 });
    expect(fetchMock).toHaveBeenCalledTimes(2); // original + refresh, sin reintento
  });

  it("no intenta refrescar en los propios endpoints de auth", async () => {
    fetchMock.mockResolvedValueOnce(res(401, { code: "invalid_credentials" }));
    await expect(post("/admin/auth/login", { email: "a", password: "b" })).rejects.toMatchObject({
      status: 401,
      code: "invalid_credentials",
    });
    expect(fetchMock).toHaveBeenCalledTimes(1); // no refresh aunque haya token
  });

  it("normaliza problem+json a ApiError con code y detail", async () => {
    fetchMock.mockResolvedValueOnce(res(422, { code: "missing_fields", detail: "x required" }));
    const err = await post("/admin/apps", {}).catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 422, code: "missing_fields", message: "x required" });
  });

  it("normaliza un 5xx con cuerpo no-JSON (HTML) a ApiError, sin SyntaxError", async () => {
    fetchMock.mockResolvedValueOnce({
      status: 502,
      ok: false,
      statusText: "Bad Gateway",
      text: async () => "<html><body>502 Bad Gateway</body></html>",
      json: async () => {
        throw new Error("no json");
      },
    } as unknown as Response);
    const err = await get("/admin/x").catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 502, code: "error" });
  });

  it("devuelve undefined en 204 sin intentar parsear", async () => {
    fetchMock.mockResolvedValueOnce(res(204));
    const out = await del("/admin/x/1");
    expect(out).toBeUndefined();
  });

  it("invoca onUnauthorized cuando el 401 persiste tras el refresh (sesión muerta)", async () => {
    fetchMock
      .mockResolvedValueOnce(res(401, { code: "invalid_token" })) // original
      .mockResolvedValueOnce(res(401)); // refresh falla
    const onUnauth = vi.fn();
    setOnUnauthorized(onUnauth);

    await expect(get("/admin/x")).rejects.toMatchObject({ status: 401 });
    expect(onUnauth).toHaveBeenCalledTimes(1);
    setOnUnauthorized(null);
  });

  it("NO invoca onUnauthorized si el refresh recupera la sesión", async () => {
    fetchMock
      .mockResolvedValueOnce(res(401, { code: "token_expired" }))
      .mockResolvedValueOnce(res(200, { accessToken: "n" }))
      .mockResolvedValueOnce(res(200, { ok: true }));
    const onUnauth = vi.fn();
    setOnUnauthorized(onUnauth);

    await get("/admin/x");
    expect(onUnauth).not.toHaveBeenCalled();
    setOnUnauthorized(null);
  });

  it("NO invoca onUnauthorized en un 401 de los endpoints de auth (login fallido)", async () => {
    fetchMock.mockResolvedValueOnce(res(401, { code: "invalid_credentials" }));
    const onUnauth = vi.fn();
    setOnUnauthorized(onUnauth);

    await expect(post("/admin/auth/login", {})).rejects.toMatchObject({ status: 401 });
    expect(onUnauth).not.toHaveBeenCalled();
    setOnUnauthorized(null);
  });
});
