import { Input, Select } from "../ui";
import type { VariableSpec } from "../pages/tabs/TemplateBuilder";

const VAR_TYPES = ["string", "number", "boolean"] as const;

// VariablesPanel: gestión de las variables detectadas en la plantilla.
// Para cada «{{variable}}» permite fijar su tipo, si es obligatoria, el valor
// con el que se renderiza la vista previa (valor de prueba) y una descripción.
// Compartido por el editor de email y por los canales SMS/PUSH/IN_APP.
export function VariablesPanel({
  detected,
  specs,
  onChange,
  testValue,
  onTestChange,
}: {
  detected: string[];
  specs: Record<string, VariableSpec>;
  onChange: (v: Record<string, VariableSpec>) => void;
  testValue: (name: string) => string;
  onTestChange: (name: string, val: string) => void;
}) {
  const setSpec = (name: string, patch: Partial<VariableSpec>) => onChange({ ...specs, [name]: { ...specs[name], ...patch } });
  return (
    <div className="rounded-xl border border-border bg-card p-3">
      <p className="mb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">Variables</p>
      <p className="mb-2 text-xs text-muted-foreground">Se detectan de los {`{{…}}`} usados. Marca su tipo, si son obligatorias y el valor con el que se ve en la vista previa.</p>
      {detected.length === 0 ? (
        <p className="text-sm text-muted-foreground">— ninguna todavía —</p>
      ) : (
        <div className="space-y-2">
          {detected.map((v) => {
            const spec = specs[v] ?? {};
            const required = spec.required !== false;
            return (
              <div key={v} className="@container space-y-2 rounded-lg border border-border p-2">
                {/* Fila 1: metadatos (código · tipo · obligatoria). Se apila según el
                    ancho del CONTENEDOR (container query): estrecho en el inspector
                    del builder, en dos filas horizontales en columnas anchas. */}
                <div className="flex flex-col gap-2 @sm:flex-row @sm:items-center">
                  <code className="shrink-0 truncate rounded bg-primary/10 px-1.5 py-0.5 font-mono text-xs text-primary @sm:w-40" title={`{{${v}}}`}>{`{{${v}}}`}</code>
                  <Select className="h-8 @sm:w-32" aria-label={`Tipo de ${v}`} value={spec.type || "string"} onChange={(e) => setSpec(v, { type: e.target.value })}>
                    {VAR_TYPES.map((t) => (
                      <option key={t} value={t}>
                        {t}
                      </option>
                    ))}
                  </Select>
                  <label className="flex shrink-0 items-center gap-1.5 text-xs text-muted-foreground">
                    <input type="checkbox" className="size-3.5 rounded border-input accent-primary" checked={required} onChange={(e) => setSpec(v, { required: e.target.checked })} />
                    Obligatoria
                  </label>
                </div>
                {/* Fila 2: valores (valor de prueba · descripción). */}
                <div className="flex flex-col gap-2 @sm:flex-row">
                  <Input className="h-8 @sm:flex-1" aria-label={`Valor de prueba de ${v}`} placeholder="Valor de prueba" value={testValue(v)} onChange={(e) => onTestChange(v, e.target.value)} />
                  <Input className="h-8 @sm:flex-1" aria-label={`Descripción de ${v}`} placeholder="Descripción (opcional)" value={spec.description || ""} onChange={(e) => setSpec(v, { description: e.target.value })} />
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

export default VariablesPanel;
