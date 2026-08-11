import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

// Mock del cliente: login() llama a post() y a tokens; aislamos la lógica de auth.
vi.mock("../api/client", () => ({
  post: vi.fn(),
  tokens: { set: vi.fn(), clear: vi.fn() },
  setOnUnauthorized: vi.fn(),
}));

import { post, tokens } from "../api/client";
import { AuthProvider, useAuth } from "./auth";

const mockedPost = vi.mocked(post);

// Consumer expone el estado y dispara login/logout para el test.
function Consumer() {
  const { admin, login, logout } = useAuth();
  return (
    <div>
      <span data-testid="email">{admin?.email ?? "anon"}</span>
      <button onClick={() => void login("a@b.c", "pw")}>login</button>
      <button onClick={() => logout()}>logout</button>
    </div>
  );
}

function renderAuth() {
  return render(
    <AuthProvider>
      <Consumer />
    </AuthProvider>,
  );
}

beforeEach(() => {
  localStorage.clear();
  vi.clearAllMocks();
});

describe("AuthProvider", () => {
  it("un qbn_admin corrupto no tumba la app y se descarta (M12)", () => {
    localStorage.setItem("qbn_admin", "{esto no es json");
    renderAuth(); // no debería lanzar
    expect(screen.getByTestId("email").textContent).toBe("anon");
    expect(localStorage.getItem("qbn_admin")).toBeNull(); // se limpió el valor corrupto
  });

  it("carga el admin válido persistido", () => {
    localStorage.setItem("qbn_admin", JSON.stringify({ id: "1", role: "SUPER_ADMIN", email: "x@y.z" }));
    renderAuth();
    expect(screen.getByTestId("email").textContent).toBe("x@y.z");
  });

  it("login guarda tokens, persiste el admin (con email del formulario) y actualiza el estado", async () => {
    mockedPost.mockResolvedValue({ accessToken: "a", admin: { id: "1", role: "APP_ADMIN" } });
    renderAuth();

    await userEvent.click(screen.getByText("login"));

    expect(mockedPost).toHaveBeenCalledWith("/admin/auth/login", { email: "a@b.c", password: "pw" });
    // El refresh token lo fija el servidor como cookie; el cliente solo el access.
    expect(tokens.set).toHaveBeenCalledWith("a");
    expect(await screen.findByText("a@b.c")).toBeInTheDocument();
    const stored = JSON.parse(localStorage.getItem("qbn_admin")!);
    expect(stored.email).toBe("a@b.c");
    expect(stored.role).toBe("APP_ADMIN");
  });

  it("logout limpia tokens, el admin persistido y el estado", async () => {
    localStorage.setItem("qbn_admin", JSON.stringify({ id: "1", role: "SUPER_ADMIN", email: "e@x.z" }));
    renderAuth();
    expect(screen.getByTestId("email").textContent).toBe("e@x.z");

    await userEvent.click(screen.getByText("logout"));

    // Avisa al servidor (revoca + borra cookie) y limpia el estado local.
    expect(mockedPost).toHaveBeenCalledWith("/admin/auth/logout");
    expect(tokens.clear).toHaveBeenCalled();
    expect(localStorage.getItem("qbn_admin")).toBeNull();
    expect(screen.getByTestId("email").textContent).toBe("anon");
  });
});
