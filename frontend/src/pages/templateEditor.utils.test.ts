import { describe, it, expect } from "vitest";
import { actionSection, buildTemplateBody, defaultStructure, filterBasesForMode, formatHtml, resolveBaseChoice, sampleValue, specsFromSchema, structureHasBlocks, textSection, toStructure } from "./templateEditor.utils";
import type { Structure } from "./tabs/TemplateBuilder";

const baseArgs = { key: "welcome", channel: "EMAIL", name: "N", subject: "S", structure: { rows: [] } as Structure, body: "<p>x</p>", baseTemplateId: "", variables: {} };

describe("buildTemplateBody", () => {
  it("editar + visual → kind 'builder', con structure y SIN body (bug de revert)", () => {
    const b = buildTemplateBody({ ...baseArgs, editing: true, htmlMode: false });
    expect(b.kind).toBe("builder");
    expect(b).toHaveProperty("structure");
    expect(b).not.toHaveProperty("body");
  });
  it("editar + html → kind 'html', con body y structure (conservada)", () => {
    const b = buildTemplateBody({ ...baseArgs, editing: true, htmlMode: true });
    expect(b.kind).toBe("html");
    expect(b.body).toBe("<p>x</p>");
    expect(b).toHaveProperty("structure");
  });
  it("nuevo + visual (sin base) → structure, sin baseTemplateId ni kind html", () => {
    const b = buildTemplateBody({ ...baseArgs, editing: false, htmlMode: false });
    expect(b).toHaveProperty("structure");
    expect(b).not.toHaveProperty("baseTemplateId");
    expect(b.kind).toBeUndefined();
    expect(b.key).toBe("welcome");
  });
  it("nuevo + visual + clon de base → baseTemplateId, sin structure ni body", () => {
    const b = buildTemplateBody({ ...baseArgs, editing: false, htmlMode: false, baseTemplateId: "base-1" });
    expect(b.baseTemplateId).toBe("base-1");
    expect(b).not.toHaveProperty("structure");
    expect(b).not.toHaveProperty("body");
  });
  it("nuevo + html → kind html + body", () => {
    const b = buildTemplateBody({ ...baseArgs, editing: false, htmlMode: true });
    expect(b.kind).toBe("html");
    expect(b.body).toBe("<p>x</p>");
  });
  it("incluye variables solo si hay alguna", () => {
    expect(buildTemplateBody({ ...baseArgs, editing: false, htmlMode: false })).not.toHaveProperty("variables");
    const withVars = buildTemplateBody({ ...baseArgs, editing: false, htmlMode: false, variables: { firstName: { type: "string" } } });
    expect(withVars).toHaveProperty("variables");
  });
});

describe("filterBasesForMode", () => {
  const bases = [
    { id: "1", channel: "EMAIL", kind: "builder" },
    { id: "2", channel: "EMAIL", kind: "html" },
    { id: "3", channel: "SMS", kind: "builder" },
  ];
  it("visual → solo las de bloques del canal", () => {
    expect(filterBasesForMode(bases, "EMAIL", "visual").map((b) => b.id)).toEqual(["1"]);
  });
  it("html → solo las html del canal", () => {
    expect(filterBasesForMode(bases, "EMAIL", "html").map((b) => b.id)).toEqual(["2"]);
  });
});

describe("resolveBaseChoice", () => {
  it("base html → precarga body y subject, sin baseTemplateId", () => {
    const c = resolveBaseChoice("id1", { kind: "html", body: "<p>hola</p>", subject: "Asunto" });
    expect(c.baseTemplateId).toBe("");
    expect(c.body).toBe("<p>hola</p>");
    expect(c.subject).toBe("Asunto");
  });
  it("base de bloques → clon en servidor (baseTemplateId), body vacío", () => {
    const c = resolveBaseChoice("id2", { kind: "builder" });
    expect(c.baseTemplateId).toBe("id2");
    expect(c.body).toBe("");
    expect(c.subject).toBeUndefined();
  });
});

describe("sampleValue", () => {
  it("da un logo (data URI) para variables de logo", () => {
    expect(sampleValue("logoUrl").startsWith("data:image")).toBe(true);
  });
  it("da el azul de la marca para variables de color", () => {
    expect(sampleValue("primaryColor")).toBe("#0ea5e9");
  });
  it("prioriza logo sobre url (logo lleva 'url' en el nombre)", () => {
    expect(sampleValue("logoUrl").startsWith("data:image")).toBe(true);
  });
  it("heurísticas por nombre", () => {
    expect(sampleValue("actionUrl")).toBe("https://ejemplo.com");
    expect(sampleValue("email")).toBe("correo@ejemplo.com");
    expect(sampleValue("firstName")).toBe("Jane");
    expect(sampleValue("otpCode")).toBe("123456");
    expect(sampleValue("appName")).toBe("Acme");
    expect(sampleValue("year")).toBe("2026");
    expect(sampleValue("cualquiera")).toBe("ejemplo");
  });
});

describe("structureHasBlocks", () => {
  it("true si hay bloques en filas/columnas", () => {
    const s: Structure = { rows: [{ id: "r", columns: [{ id: "c", width: 1, blocks: [{ id: "b", type: "text", props: {} }] }] }] };
    expect(structureHasBlocks(s)).toBe(true);
  });
  it("false si las columnas están vacías", () => {
    const s: Structure = { rows: [{ id: "r", columns: [{ id: "c", width: 1, blocks: [] }] }] };
    expect(structureHasBlocks(s)).toBe(false);
  });
  it("true/false según sections (formato plano)", () => {
    expect(structureHasBlocks({ sections: [{ id: "s", type: "text", props: {} }] })).toBe(true);
    expect(structureHasBlocks({ sections: [] })).toBe(false);
  });
});

describe("defaultStructure", () => {
  it("EMAIL: filas con cabecera/texto/botón/pie, color azul y logo", () => {
    const s = defaultStructure("EMAIL");
    const blocks = s.rows![0].columns[0].blocks;
    expect(blocks.map((b) => b.type)).toEqual(["header", "text", "button", "footer"]);
    expect(s.theme?.primaryColor).toBe("#0ea5e9");
    expect(blocks[0].props.logoUrl?.startsWith("data:image")).toBe(true);
  });
  it("SMS/PUSH usan lista plana de sections", () => {
    expect(defaultStructure("SMS").sections?.[0].props.text).toContain("{{code}}");
    expect(defaultStructure("PUSH").sections?.length).toBe(1);
  });
});

describe("toStructure", () => {
  it("EMAIL desde sections legado → lo envuelve en fila/columna", () => {
    const s = toStructure({ sections: [{ type: "text", props: { text: "x" } }] }, "EMAIL");
    expect(s.rows).toHaveLength(1);
    expect(s.rows![0].columns[0].blocks[0].type).toBe("text");
  });
  it("EMAIL desde rows → conserva filas y normaliza width", () => {
    const s = toStructure({ rows: [{ columns: [{ width: 2, blocks: [{ type: "header", props: {} }] }] }] }, "EMAIL");
    expect(s.rows![0].columns[0].width).toBe(2);
  });
  it("no-EMAIL → mantiene sections planas", () => {
    const s = toStructure({ sections: [{ type: "text", props: { text: "hola" } }] }, "SMS");
    expect(s.sections).toHaveLength(1);
    expect(s.rows).toBeUndefined();
  });
  it("entrada nula no revienta", () => {
    expect(() => toStructure(null, "EMAIL")).not.toThrow();
    expect(() => toStructure(undefined, "SMS")).not.toThrow();
  });
  it("asigna id a las secciones sin id", () => {
    const s = toStructure({ sections: [{ type: "text", props: {} }] }, "SMS");
    expect(s.sections![0].id).toBeTruthy();
  });
});

describe("specsFromSchema", () => {
  it("construye specs desde properties + required", () => {
    const specs = specsFromSchema({ properties: { a: { type: "string" }, b: { type: "number", description: "d" } }, required: ["a"] });
    expect(specs.a.required).toBe(true);
    expect(specs.b.required).toBe(false);
    expect(specs.b.type).toBe("number");
    expect(specs.b.description).toBe("d");
  });
  it("entrada vacía → objeto vacío", () => {
    expect(specsFromSchema(null)).toEqual({});
    expect(specsFromSchema({})).toEqual({});
  });
});

describe("formatHtml", () => {
  it("indenta HTML en una sola línea (mete saltos de línea)", () => {
    const out = formatHtml("<div><p>hola</p></div>");
    expect(out).toContain("\n");
  });
  it("conserva las variables {{ }} y no revienta con vacío", () => {
    expect(formatHtml("<p>{{firstName}}</p>")).toContain("{{firstName}}");
    expect(formatHtml("")).toBe("");
    expect(formatHtml("   ")).toBe("   ");
  });
});

describe("textSection / actionSection", () => {
  const s: Structure = { sections: [{ id: "1", type: "text", props: { text: "hola" } }, { id: "2", type: "button", props: { url: "u" } }] };
  it("encuentran la sección de texto y la de botón", () => {
    expect(textSection(s)?.props.text).toBe("hola");
    expect(actionSection(s)?.props.url).toBe("u");
  });
  it("devuelven undefined si no hay sections", () => {
    expect(textSection({ rows: [] })).toBeUndefined();
  });
});
