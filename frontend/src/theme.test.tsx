import { describe, it, expect, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ThemeProvider, ThemeToggle } from "./theme";

beforeEach(() => {
  localStorage.clear();
  document.documentElement.classList.remove("dark");
});

describe("ThemeToggle", () => {
  it("arranca en claro (sin preferencia guardada) y alterna el tema oscuro persistiéndolo", async () => {
    const user = userEvent.setup();
    render(
      <ThemeProvider>
        <ThemeToggle />
      </ThemeProvider>,
    );

    expect(document.documentElement).not.toHaveClass("dark");

    await user.click(screen.getByRole("button", { name: /tema oscuro/i }));
    expect(document.documentElement).toHaveClass("dark");
    expect(localStorage.getItem("qbn-theme")).toBe("dark");

    await user.click(screen.getByRole("button", { name: /tema claro/i }));
    expect(document.documentElement).not.toHaveClass("dark");
    expect(localStorage.getItem("qbn-theme")).toBe("light");
  });

  it("respeta el tema guardado en localStorage", () => {
    localStorage.setItem("qbn-theme", "dark");
    render(
      <ThemeProvider>
        <ThemeToggle />
      </ThemeProvider>,
    );
    expect(document.documentElement).toHaveClass("dark");
  });
});
