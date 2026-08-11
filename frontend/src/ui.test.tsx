import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { Table } from "./ui";

describe("Table", () => {
  it("renderiza los encabezados", () => {
    render(
      <Table head={["Nombre", "Estado"]}>
        <tr>
          <td>Acme</td>
          <td>Activo</td>
        </tr>
      </Table>,
    );
    expect(screen.getByText("Nombre")).toBeInTheDocument();
    expect(screen.getByText("Estado")).toBeInTheDocument();
  });

  it("etiqueta cada celda con su encabezado (data-label) para la vista móvil", () => {
    render(
      <Table head={["Nombre", "Estado"]}>
        <tr>
          <td>Acme</td>
          <td>Activo</td>
        </tr>
      </Table>,
    );
    expect(screen.getByText("Acme")).toHaveAttribute("data-label", "Nombre");
    expect(screen.getByText("Activo")).toHaveAttribute("data-label", "Estado");
  });
});
