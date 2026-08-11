import { describe, it, expect } from "vitest";
import { detectVariables, detectVariablesInText, uid, type Structure } from "./TemplateBuilder";

function struct(props: Record<string, string>): Structure {
  return { sections: [{ id: "s1", type: "text", props }] };
}

describe("detectVariables", () => {
  it("extrae variables del asunto y de las props de las secciones", () => {
    const vars = detectVariables(struct({ text: "Hola {{firstName}}", url: "{{actionUrl}}" }), "Bienvenida {{appName}}");
    expect(vars).toContain("firstName");
    expect(vars).toContain("actionUrl");
    expect(vars).toContain("appName");
  });

  it("deduplica variables repetidas", () => {
    const vars = detectVariables(struct({ a: "{{x}} y {{x}}", b: "{{x}}" }), "{{x}}");
    expect(vars.filter((v) => v === "x")).toHaveLength(1);
  });

  it("ignora helpers de bloque ({{#if}} / {{/if}})", () => {
    const vars = detectVariables(struct({ text: "{{#if cond}}hola{{/if}}" }), "");
    expect(vars).not.toContain("#if");
    expect(vars).not.toContain("/if");
  });

  it("devuelve vacío si no hay variables", () => {
    expect(detectVariables(struct({ text: "texto plano sin variables" }), "asunto")).toHaveLength(0);
  });

  it("toma solo el nombre raíz de una ruta con punto", () => {
    const vars = detectVariables(struct({ text: "{{user.name}}" }), "");
    expect(vars).toContain("user");
  });
});

describe("detectVariablesInText", () => {
  it("extrae variables de un texto plano (HTML crudo del modo avanzado)", () => {
    const vars = detectVariablesInText(`<p>Hola {{firstName}}</p><a href="{{actionUrl}}">ir</a>`);
    expect(vars).toContain("firstName");
    expect(vars).toContain("actionUrl");
  });
  it("ignora helpers y deduplica", () => {
    const vars = detectVariablesInText("{{#if x}}{{code}}{{/if}} {{code}}");
    expect(vars).toContain("code");
    expect(vars).not.toContain("#if");
    expect(vars.filter((v) => v === "code")).toHaveLength(1);
  });
  it("vacío si no hay nada", () => {
    expect(detectVariablesInText("sin variables")).toHaveLength(0);
    expect(detectVariablesInText("")).toHaveLength(0);
  });
});

describe("uid", () => {
  it("genera ids no vacíos y únicos", () => {
    const a = uid();
    const b = uid();
    expect(a).toBeTruthy();
    expect(a).not.toBe(b);
  });
});
