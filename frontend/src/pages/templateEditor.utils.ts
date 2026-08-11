// Funciones puras del editor de plantillas, extraídas de TemplateEditor.tsx para
// poder testearlas de forma aislada (sin montar el componente).
import { html as beautifyHtml } from "js-beautify";
import { BRAND_LOGO } from "@/lib/brandLogo";
import { uid, type Structure, type VariableSpec } from "./tabs/TemplateBuilder";

// Esqueleto de partida cuando se abre el modo HTML sin nada que sembrar.
export const DEFAULT_HTML = `<!DOCTYPE html>
<html>
  <body style="margin:0;background:#f4f4f4;font-family:Arial,Helvetica,sans-serif;">
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f4f4f4;">
      <tr><td align="center" style="padding:24px;">
        <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width:600px;width:100%;background:#fff;border-radius:8px;">
          <tr><td style="text-align:center;padding:24px 24px 8px;"><img src="${BRAND_LOGO}" alt="{{appName}}" style="max-height:44px;max-width:200px;"></td></tr>
          <tr><td style="padding:8px 32px 32px;">
            <h1 style="margin:0 0 12px;font-size:22px;color:#111;">Hola {{firstName}}</h1>
            <p style="margin:0;color:#444;font-size:15px;line-height:1.6;">Escribe aquí tu contenido. Usa {{variables}} para insertar datos.</p>
          </td></tr>
        </table>
      </td></tr>
    </table>
  </body>
</html>`;

// formatHtml indenta/embellece el HTML (para que no salga en una sola línea).
export function formatHtml(src: string): string {
  if (!src.trim()) return src;
  try {
    return beautifyHtml(src, {
      indent_size: 2,
      wrap_line_length: 0,
      preserve_newlines: true,
      max_preserve_newlines: 1,
      indent_inner_html: true,
      // No tocar el contenido de <style>/<script> para no romper el CSS.
      content_unformatted: ["style"],
    });
  } catch {
    return src;
  }
}

// structureHasBlocks indica si la estructura de bloques tiene contenido real
// (para saber si se puede "volver a visual" desde el modo html).
export function structureHasBlocks(s: Structure): boolean {
  if (s.rows?.some((r) => r.columns?.some((c) => c.blocks?.length))) return true;
  return (s.sections?.length ?? 0) > 0;
}

// Estructura inicial por canal. EMAIL usa el modelo Filas→Columnas→Bloques; el
// resto usa la lista plana (sections) que consume ChannelContentEditor.
export function defaultStructure(channel: string): Structure {
  if (channel === "EMAIL") {
    return {
      theme: { primaryColor: "#0ea5e9", fontFamily: "" },
      rows: [
        {
          id: uid(),
          columns: [
            {
              id: uid(),
              width: 1,
              blocks: [
                { id: uid(), type: "header", props: { title: "{{appName}}", logoUrl: BRAND_LOGO } },
                { id: uid(), type: "text", props: { text: "Hola {{firstName}}, te damos la bienvenida." } },
                { id: uid(), type: "button", props: { text: "Empezar", url: "{{actionUrl}}" } },
                { id: uid(), type: "footer", props: { text: "© {{year}} {{appName}}" } },
              ],
            },
          ],
        },
      ],
    };
  }
  if (channel === "SMS") {
    return { theme: {}, sections: [{ id: uid(), type: "text", props: { text: "Tu código es {{code}}" } }] };
  }
  // PUSH / IN_APP: cuerpo + acción opcional (el título va en «subject»).
  return { theme: {}, sections: [{ id: uid(), type: "text", props: { text: "Hola {{firstName}} 👋" } }] };
}

const toSection = (sec: unknown) => {
  const x = (sec ?? {}) as { id?: string; type?: string; props?: Record<string, string> };
  return { id: x.id ?? uid(), type: x.type ?? "text", props: x.props ?? {} };
};

// toStructure normaliza la estructura guardada. Para EMAIL produce filas/columnas
// (envolviendo el formato plano legado en una fila de una columna); para el resto
// mantiene la lista plana.
export function toStructure(raw: unknown, channel: string): Structure {
  const s = (raw ?? {}) as { theme?: Structure["theme"]; rows?: unknown; sections?: unknown };
  const theme = s.theme ?? {};
  if (channel !== "EMAIL") {
    const sections = Array.isArray(s.sections) ? s.sections.map(toSection) : [];
    return { theme, sections };
  }
  if (Array.isArray(s.rows) && s.rows.length) {
    const rows = s.rows.map((r) => {
      const row = (r ?? {}) as { id?: string; columns?: unknown };
      const columns = Array.isArray(row.columns)
        ? row.columns.map((c) => {
            const col = (c ?? {}) as { id?: string; width?: number; blocks?: unknown };
            return { id: col.id ?? uid(), width: col.width && col.width > 0 ? col.width : 1, blocks: Array.isArray(col.blocks) ? col.blocks.map(toSection) : [] };
          })
        : [{ id: uid(), width: 1, blocks: [] }];
      return { id: row.id ?? uid(), columns };
    });
    return { theme, rows };
  }
  // Legado plano → una fila de una columna.
  const blocks = Array.isArray(s.sections) ? s.sections.map(toSection) : [];
  return { theme, rows: [{ id: uid(), columns: [{ id: uid(), width: 1, blocks }] }] };
}

export function specsFromSchema(raw: unknown): Record<string, VariableSpec> {
  const schema = (raw ?? {}) as { properties?: Record<string, { type?: string; description?: string }>; required?: string[] };
  const props = schema.properties ?? {};
  const required = new Set(schema.required ?? []);
  const out: Record<string, VariableSpec> = {};
  for (const [name, p] of Object.entries(props)) {
    out[name] = { type: p.type || "string", description: p.description || "", required: required.has(name) };
  }
  return out;
}

// Valor de ejemplo para una variable (vista previa en vivo).
export function sampleValue(name: string): string {
  const n = name.toLowerCase();
  if (n.includes("logo")) return BRAND_LOGO;
  if (n.includes("color")) return "#0ea5e9";
  if (n.includes("url") || n.includes("link")) return "https://ejemplo.com";
  if (n.includes("email") || n.includes("correo")) return "correo@ejemplo.com";
  // "app" va antes que "name": si no, {{appName}} caería en la rama de nombre.
  if (n.includes("app")) return "Acme";
  if (n.includes("first") || n.includes("name") || n.includes("nombre")) return "Jane";
  if (n.includes("year") || n.includes("año") || n.includes("anio")) return "2026";
  if (n.includes("date") || n.includes("fecha")) return "2026-06-26";
  if (n.includes("code") || n.includes("codigo") || n.includes("otp")) return "123456";
  if (n.includes("amount") || n.includes("total") || n.includes("price")) return "49,99 €";
  return "ejemplo";
}

// --- Helpers de estructura para editores de solo texto ---
export const textSection = (s: Structure) => s.sections?.find((x) => x.type === "text");
export const actionSection = (s: Structure) => s.sections?.find((x) => x.type === "button");

export type EditorMode = "visual" | "html";

// buildTemplateBody arma el cuerpo de la petición de guardado (crear o versionar).
// Envía `kind` SIEMPRE explícito al editar: si no, al revertir html→visual el
// backend heredaría "html" del actual y descartaría la edición visual.
export function buildTemplateBody(opts: {
  editing: boolean;
  htmlMode: boolean;
  key: string;
  channel: string;
  name: string;
  subject: string;
  structure: Structure;
  body: string;
  baseTemplateId: string;
  variables: Record<string, VariableSpec>; // ya filtradas a las detectadas
}): Record<string, unknown> {
  const varsField = Object.keys(opts.variables).length ? { variables: opts.variables } : {};
  if (opts.editing) {
    const b: Record<string, unknown> = {
      name: opts.name,
      subject: opts.subject,
      kind: opts.htmlMode ? "html" : "builder",
      structure: opts.structure, // se conserva siempre (para "volver a visual")
      ...varsField,
    };
    if (opts.htmlMode) b.body = opts.body;
    return b;
  }
  const b: Record<string, unknown> = { key: opts.key, channel: opts.channel, name: opts.name, subject: opts.subject };
  if (opts.htmlMode) {
    b.kind = "html";
    b.body = opts.body;
    Object.assign(b, varsField);
  } else if (opts.baseTemplateId) {
    b.baseTemplateId = opts.baseTemplateId; // clon en servidor (bases de bloques)
  } else {
    b.structure = opts.structure;
    Object.assign(b, varsField);
  }
  return b;
}

// filterBasesForMode filtra las plantillas base por canal y modo: bloques en visual,
// HTML en avanzado (el desplegable "Partir de una base" solo muestra las del modo).
export function filterBasesForMode<T extends { channel: string; kind?: string }>(bases: T[], channel: string, mode: EditorMode): T[] {
  return bases.filter((b) => b.channel === channel && (mode === "html" ? b.kind === "html" : b.kind !== "html"));
}

// resolveBaseChoice decide qué cargar al elegir una base: las HTML precargan el body
// (y su subject); las de bloques se clonan en el servidor por baseTemplateId.
export function resolveBaseChoice(id: string, base?: { kind?: string; body?: string; subject?: string }): { baseTemplateId: string; body: string; subject?: string } {
  if (base?.kind === "html") {
    return { baseTemplateId: "", body: base.body ?? "", subject: base.subject };
  }
  return { baseTemplateId: id, body: "" };
}
