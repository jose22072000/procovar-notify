import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { VariablesPanel } from "./VariablesPanel";
import type { VariableSpec } from "../pages/tabs/TemplateBuilder";

function setup(detected: string[], specs: Record<string, VariableSpec> = {}) {
  const onChange = vi.fn();
  const onTestChange = vi.fn();
  const testValue = (n: string) => (n === "firstName" ? "Jane" : "");
  render(<VariablesPanel detected={detected} specs={specs} onChange={onChange} testValue={testValue} onTestChange={onTestChange} />);
  return { onChange, onTestChange };
}

describe("VariablesPanel", () => {
  it("muestra un aviso cuando no hay variables", () => {
    setup([]);
    expect(screen.getByText(/ninguna todav/i)).toBeInTheDocument();
  });

  it("renderiza una fila por variable con su valor de prueba", () => {
    setup(["firstName"]);
    expect(screen.getByText("{{firstName}}")).toBeInTheDocument();
    expect(screen.getByLabelText(/Valor de prueba de firstName/i)).toHaveValue("Jane");
  });

  it("editar el valor de prueba dispara onTestChange", () => {
    const { onTestChange } = setup(["firstName"]);
    fireEvent.change(screen.getByLabelText(/Valor de prueba de firstName/i), { target: { value: "Marta" } });
    expect(onTestChange).toHaveBeenCalledWith("firstName", "Marta");
  });

  it("editar la descripción dispara onChange con el spec actualizado", () => {
    const { onChange } = setup(["firstName"]);
    fireEvent.change(screen.getByLabelText(/Descripción de firstName/i), { target: { value: "El nombre" } });
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ firstName: expect.objectContaining({ description: "El nombre" }) }));
  });

  it("desmarcar 'Obligatoria' dispara onChange con required=false", () => {
    const { onChange } = setup(["firstName"]);
    fireEvent.click(screen.getByRole("checkbox"));
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ firstName: expect.objectContaining({ required: false }) }));
  });
});
