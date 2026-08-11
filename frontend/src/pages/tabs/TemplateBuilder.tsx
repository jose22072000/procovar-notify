// Tipos y utilidades compartidas del builder de plantillas. El modelo del email
// es Filas → Columnas → Bloques; también se admite el formato legado plano
// (sections) para plantillas antiguas. El editor visual vive en EmailBuilder.tsx.

export interface Section {
  id: string;
  type: string;
  props: Record<string, string>;
}
export interface Column {
  id: string;
  width: number; // peso relativo (1, 2…) para repartir el ancho
  blocks: Section[];
}
export interface Row {
  id: string;
  columns: Column[];
}
export interface Structure {
  theme?: { primaryColor?: string; fontFamily?: string; fontImport?: string };
  rows?: Row[];
  sections?: Section[]; // legado (lista plana)
}

// VariableSpec refleja el contrato del backend: afina una variable derivada.
export interface VariableSpec {
  required?: boolean;
  type?: string; // string | number | boolean
  description?: string;
}

// Preview es el resultado del endpoint de vista previa (compartido por los editores).
export interface Preview {
  subject: string;
  html: string; // renderizado con datos de prueba (para el iframe)
  body?: string; // compilado sin renderizar (con {{vars}}); para "pasar a HTML"
  missingVariables: string[] | null;
}

// uid genera un id único para secciones/bloques (usa crypto.randomUUID si existe).
export const uid = (): string => (crypto.randomUUID ? crypto.randomUUID() : `s${Date.now()}${Math.floor(Math.random() * 1e6)}`);

// Fuentes del sistema (stacks web-safe).
export const SYSTEM_FONTS: { label: string; value: string }[] = [
  { label: "Sistema (por defecto)", value: "" },
  { label: "Arial", value: "Arial, sans-serif" },
  { label: "Helvetica", value: "Helvetica, Arial, sans-serif" },
  { label: "Verdana", value: "Verdana, sans-serif" },
  { label: "Tahoma", value: "Tahoma, sans-serif" },
  { label: "Trebuchet MS", value: "'Trebuchet MS', sans-serif" },
  { label: "Georgia", value: "Georgia, serif" },
  { label: "Times New Roman", value: "'Times New Roman', serif" },
  { label: "Courier New (monoespaciada)", value: "'Courier New', monospace" },
];

// Web fonts (Google Fonts). Al elegirlas se añade su <link> en el email.
export const WEB_FONTS: { label: string; value: string; import: string }[] = [
  { label: "Inter (web)", value: "'Inter', Arial, sans-serif", import: "https://fonts.googleapis.com/css2?family=Inter:wght@400;600;700&display=swap" },
  { label: "Roboto (web)", value: "'Roboto', Arial, sans-serif", import: "https://fonts.googleapis.com/css2?family=Roboto:wght@400;500;700&display=swap" },
  { label: "Open Sans (web)", value: "'Open Sans', Arial, sans-serif", import: "https://fonts.googleapis.com/css2?family=Open+Sans:wght@400;600;700&display=swap" },
  { label: "Lato (web)", value: "'Lato', Arial, sans-serif", import: "https://fonts.googleapis.com/css2?family=Lato:wght@400;700&display=swap" },
  { label: "Montserrat (web)", value: "'Montserrat', Arial, sans-serif", import: "https://fonts.googleapis.com/css2?family=Montserrat:wght@400;600;700&display=swap" },
  { label: "Poppins (web)", value: "'Poppins', Arial, sans-serif", import: "https://fonts.googleapis.com/css2?family=Poppins:wght@400;600;700&display=swap" },
  { label: "Lora (web, serif)", value: "'Lora', Georgia, serif", import: "https://fonts.googleapis.com/css2?family=Lora:wght@400;600;700&display=swap" },
  { label: "Merriweather (web, serif)", value: "'Merriweather', Georgia, serif", import: "https://fonts.googleapis.com/css2?family=Merriweather:wght@400;700&display=swap" },
];

const FONT_IMPORT = new Map(WEB_FONTS.map((f) => [f.value, f.import] as const));

// fontImportFor devuelve la URL de import de una web font (o "" si no aplica).
export function fontImportFor(value: string): string {
  return FONT_IMPORT.get(value) ?? "";
}

// blocksOf aplana todos los bloques de una estructura (filas/columnas o legado).
export function blocksOf(structure: Structure): Section[] {
  if (structure.rows && structure.rows.length) {
    return structure.rows.flatMap((r) => r.columns.flatMap((c) => c.blocks));
  }
  return structure.sections ?? [];
}

// detectVariables extrae las variables Handlebars {{x}} del asunto y de todos los
// bloques (ignora helpers #/ /). Deduplica; toma solo el segmento raíz (user.name → user).
// scanInto acumula las variables {{...}} de un texto en el set dado.
function scanInto(found: Set<string>, text: string) {
  for (const m of text.matchAll(/\{\{\s*([^}#/][^}]*?)\s*\}\}/g)) {
    const name = m[1].trim().split(/[\s.]/)[0];
    if (name && !name.startsWith("#") && !name.startsWith("/")) found.add(name);
  }
}

// detectVariablesInText extrae variables {{...}} de un texto plano (asunto, HTML
// crudo del modo avanzado…).
export function detectVariablesInText(text: string): string[] {
  const found = new Set<string>();
  scanInto(found, text || "");
  return [...found];
}

export function detectVariables(structure: Structure, subject: string): string[] {
  const found = new Set<string>();
  scanInto(found, subject || "");
  for (const s of blocksOf(structure)) for (const v of Object.values(s.props)) scanInto(found, v || "");
  return [...found];
}
