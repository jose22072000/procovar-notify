import { useState, type ComponentType, type ReactNode } from "react";
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  closestCorners,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from "@dnd-kit/core";
import { SortableContext, arrayMove, useSortable, verticalListSortingStrategy } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import {
  AlignCenter,
  AlignJustify,
  AlignLeft,
  AlignRight,
  Columns2,
  Columns3,
  Copy,
  GripVertical,
  Heading,
  Image as ImageIcon,
  LayoutTemplate,
  Minus,
  Monitor,
  MousePointerClick,
  MoveVertical,
  PanelBottomClose,
  Plus,
  RectangleHorizontal,
  Rows3,
  Smartphone,
  Tablet,
  Trash2,
  Type,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { Input, Label, Select } from "../../ui";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Check } from "lucide-react";
import { VariableField } from "../../components/VariableField";
import { VariablesPanel } from "../../components/VariablesPanel";
import { SYSTEM_FONTS, WEB_FONTS, detectVariables, fontImportFor, uid, type Column, type Preview, type Row, type Section, type Structure, type VariableSpec } from "./TemplateBuilder";

const ELEMENTS: { type: string; label: string; icon: ComponentType<{ className?: string }> }[] = [
  { type: "header", label: "Cabecera", icon: Heading },
  { type: "text", label: "Texto", icon: Type },
  { type: "button", label: "Botón", icon: MousePointerClick },
  { type: "image", label: "Imagen", icon: ImageIcon },
  { type: "divider", label: "Separador", icon: Minus },
  { type: "spacer", label: "Espacio", icon: MoveVertical },
  { type: "footer", label: "Pie", icon: PanelBottomClose },
];
const ELEMENT_LABEL: Record<string, string> = Object.fromEntries(ELEMENTS.map((e) => [e.type, e.label]));
const ELEMENT_ICON: Record<string, ComponentType<{ className?: string }>> = Object.fromEntries(ELEMENTS.map((e) => [e.type, e.icon]));

const DEFAULTS: Record<string, Record<string, string>> = {
  header: { title: "{{appName}}" },
  text: { text: "Escribe tu texto aquí. Usa {{variables}} para personalizar." },
  button: { text: "Empezar", url: "{{actionUrl}}" },
  image: { url: "", alt: "" },
  divider: {},
  spacer: { height: "16" },
  footer: { text: "© {{year}} {{appName}}" },
};

type FieldDef = { key: string; label: string; textarea?: boolean; variable?: boolean; number?: boolean; options?: { value: string; label: string }[]; placeholder?: string; showIf?: (props: Record<string, string>) => boolean };
const FIELDS: Record<string, FieldDef[]> = {
  header: [
    { key: "logoUrl", label: "Logo URL", placeholder: "https://…/logo.png" },
    { key: "title", label: "Título", variable: true },
  ],
  text: [{ key: "text", label: "Texto", textarea: true, variable: true }],
  button: [
    { key: "text", label: "Texto del botón", variable: true },
    { key: "url", label: "URL", variable: true, placeholder: "{{actionUrl}}" },
  ],
  image: [
    { key: "url", label: "Imagen URL", placeholder: "https://…/img.png" },
    { key: "alt", label: "Texto alternativo" },
  ],
  divider: [
    {
      key: "orientation",
      label: "Orientación",
      options: [
        { value: "horizontal", label: "Horizontal" },
        { value: "vertical", label: "Vertical" },
      ],
    },
    { key: "thickness", label: "Grosor (px)", number: true, placeholder: "1" },
    { key: "height", label: "Alto (px)", number: true, placeholder: "40", showIf: (p) => p.orientation === "vertical" },
  ],
  spacer: [{ key: "height", label: "Altura (px)", number: true, placeholder: "16" }],
  footer: [{ key: "text", label: "Texto", textarea: true, variable: true }],
};

const STYLE: Record<string, { align?: boolean; fontSize?: boolean; color?: boolean }> = {
  header: { align: true, fontSize: true, color: true },
  text: { align: true, fontSize: true, color: true },
  footer: { align: true, fontSize: true, color: true },
  button: { align: true },
  image: { align: true },
  divider: {},
  spacer: {},
};

const LAYOUTS: { key: string; label: string; widths: number[]; icon: ComponentType<{ className?: string }> }[] = [
  { key: "1", label: "1 columna", widths: [1], icon: RectangleHorizontal },
  { key: "1-1", label: "2 iguales", widths: [1, 1], icon: Columns2 },
  { key: "1-2", label: "⅓ · ⅔", widths: [1, 2], icon: Columns2 },
  { key: "2-1", label: "⅔ · ⅓", widths: [2, 1], icon: Columns2 },
  { key: "1-1-1", label: "3 iguales", widths: [1, 1, 1], icon: Columns3 },
];

const VP: Record<string, { w: number; label: string; icon: ComponentType<{ className?: string }> }> = {
  movil: { w: 390, label: "Móvil", icon: Smartphone },
  tablet: { w: 600, label: "Tablet", icon: Tablet },
  pc: { w: 800, label: "PC", icon: Monitor },
};

const ALIGNS: { value: string; icon: ComponentType<{ className?: string }>; label: string }[] = [
  { value: "left", icon: AlignLeft, label: "Izquierda" },
  { value: "center", icon: AlignCenter, label: "Centro" },
  { value: "right", icon: AlignRight, label: "Derecha" },
  { value: "justify", icon: AlignJustify, label: "Justificado" },
];

type Sel = { rowId: string; colId: string; blockId?: string } | null;

export default function EmailBuilder({
  value,
  onChange,
  subject,
  variables,
  onVariablesChange,
  preview,
  previewPaused,
  testValue,
  onTestChange,
  footer,
}: {
  value: Structure;
  onChange: (s: Structure) => void;
  subject: string;
  variables: Record<string, VariableSpec>;
  onVariablesChange: (v: Record<string, VariableSpec>) => void;
  preview: Preview | null;
  previewPaused: boolean;
  testValue: (name: string) => string;
  onTestChange: (name: string, val: string) => void;
  footer?: ReactNode;
}) {
  const [sel, setSel] = useState<Sel>(null);
  const [viewport, setViewport] = useState<keyof typeof VP>("pc");
  const [mode, setMode] = useState<"design" | "preview">("design");
  const [activeId, setActiveId] = useState<string | null>(null);
  // Punto de inserción en vivo (columna + índice) para dibujar la guía de drop.
  const [drop, setDrop] = useState<{ colId: string; index: number } | null>(null);

  const rows = value.rows ?? [];
  const theme = value.theme ?? {};
  const setRows = (next: Row[]) => onChange({ ...value, rows: next });
  const setTheme = (patch: Partial<NonNullable<Structure["theme"]>>) => onChange({ ...value, theme: { ...theme, ...patch } });

  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }));

  // --- Localizadores ---
  const findBlock = (id: string): { rowId: string; colId: string; index: number; block: Section } | null => {
    for (const r of rows) for (const c of r.columns) {
      const i = c.blocks.findIndex((b) => b.id === id);
      if (i >= 0) return { rowId: r.id, colId: c.id, index: i, block: c.blocks[i] };
    }
    return null;
  };
  const findColumn = (colId: string): { rowId: string; column: Column } | null => {
    for (const r of rows) for (const c of r.columns) if (c.id === colId) return { rowId: r.id, column: c };
    return null;
  };

  // --- Operaciones sobre filas ---
  const addRow = (widths: number[]) => {
    const row: Row = { id: uid(), columns: widths.map((w) => ({ id: uid(), width: w, blocks: [] })) };
    setRows([...rows, row]);
    setSel({ rowId: row.id, colId: row.columns[0].id });
  };
  const removeRow = (rowId: string) => {
    setRows(rows.filter((r) => r.id !== rowId));
    setSel(null);
  };
  const setRowLayout = (rowId: string, widths: number[]) => {
    setRows(
      rows.map((r) => {
        if (r.id !== rowId) return r;
        const cols: Column[] = widths.map((w, i) => ({ id: r.columns[i]?.id ?? uid(), width: w, blocks: r.columns[i]?.blocks ?? [] }));
        if (r.columns.length > widths.length) {
          const extra = r.columns.slice(widths.length).flatMap((c) => c.blocks);
          cols[cols.length - 1] = { ...cols[cols.length - 1], blocks: [...cols[cols.length - 1].blocks, ...extra] };
        }
        return { ...r, columns: cols };
      }),
    );
  };

  // --- Operaciones sobre bloques ---
  const lastColumn = (): { rowId: string; colId: string } | null => {
    if (!rows.length) return null;
    const r = rows[rows.length - 1];
    const c = r.columns[r.columns.length - 1];
    return { rowId: r.id, colId: c.id };
  };
  const addBlock = (type: string) => {
    const target = sel ? { rowId: sel.rowId, colId: sel.colId } : lastColumn();
    const block: Section = { id: uid(), type, props: { ...(DEFAULTS[type] ?? {}) } };
    if (!target) {
      const row: Row = { id: uid(), columns: [{ id: uid(), width: 1, blocks: [block] }] };
      setRows([...rows, row]);
      setSel({ rowId: row.id, colId: row.columns[0].id, blockId: block.id });
      return;
    }
    setRows(rows.map((r) => (r.id !== target.rowId ? r : { ...r, columns: r.columns.map((c) => (c.id !== target.colId ? c : { ...c, blocks: [...c.blocks, block] })) })));
    setSel({ rowId: target.rowId, colId: target.colId, blockId: block.id });
  };
  const mapCol = (rowId: string, colId: string, fn: (blocks: Section[]) => Section[]) =>
    setRows(rows.map((r) => (r.id !== rowId ? r : { ...r, columns: r.columns.map((c) => (c.id !== colId ? c : { ...c, blocks: fn(c.blocks) })) })));
  const removeBlock = (rowId: string, colId: string, id: string) => {
    mapCol(rowId, colId, (bs) => bs.filter((b) => b.id !== id));
    setSel({ rowId, colId });
  };
  const duplicateBlock = (rowId: string, colId: string, id: string) =>
    mapCol(rowId, colId, (bs) => {
      const i = bs.findIndex((b) => b.id === id);
      if (i < 0) return bs;
      const next = [...bs];
      next.splice(i + 1, 0, { ...bs[i], id: uid(), props: { ...bs[i].props } });
      return next;
    });
  const setProp = (rowId: string, colId: string, id: string, key: string, val: string) =>
    mapCol(rowId, colId, (bs) => bs.map((b) => (b.id !== id ? b : { ...b, props: { ...b.props, [key]: val } })));

  // moveBlockTo mueve un bloque a una columna/posición (misma o distinta columna).
  const moveBlockTo = (src: { rowId: string; colId: string; index: number; block: Section }, destRowId: string, destColId: string, destIndex: number) => {
    const next = rows.map((r) => ({ ...r, columns: r.columns.map((c) => ({ ...c, blocks: [...c.blocks] })) }));
    const sCol = next.find((r) => r.id === src.rowId)?.columns.find((c) => c.id === src.colId);
    const dCol = next.find((r) => r.id === destRowId)?.columns.find((c) => c.id === destColId);
    if (!sCol || !dCol) return;
    sCol.blocks.splice(src.index, 1);
    let idx = destIndex;
    if (src.colId === destColId && src.index < destIndex) idx -= 1;
    dCol.blocks.splice(idx, 0, src.block);
    setRows(next);
    setSel({ rowId: destRowId, colId: destColId, blockId: src.block.id });
  };

  // --- Drag & drop ---
  // computeBlockDrop resuelve la columna e índice de inserción a partir del
  // elemento bajo el cursor y de si el ratón está por encima/debajo de su mitad.
  const computeBlockDrop = (e: DragEndEvent): { rowId: string; colId: string; index: number } | null => {
    const { active, over } = e;
    if (!over || active.data.current?.type === "row") return null;
    const overId = String(over.id);
    if (overId.startsWith("col:")) {
      const loc = findColumn(overId.slice(4));
      return loc ? { rowId: loc.rowId, colId: loc.column.id, index: loc.column.blocks.length } : null;
    }
    if (overId.startsWith("row:")) {
      const r = rows.find((x) => x.id === overId.slice(4));
      return r ? { rowId: r.id, colId: r.columns[0].id, index: r.columns[0].blocks.length } : null;
    }
    const dst = findBlock(overId);
    if (!dst) return null;
    // Posición real del cursor = punto inicial del gesto + desplazamiento actual.
    const activator = e.activatorEvent as { clientY?: number } | null;
    const pointerY = activator && typeof activator.clientY === "number" ? activator.clientY + e.delta.y : null;
    const overRect = over.rect;
    const mid = overRect.top + overRect.height / 2;
    const after = pointerY !== null ? pointerY > mid : false;
    return { rowId: dst.rowId, colId: dst.colId, index: dst.index + (after ? 1 : 0) };
  };

  const onDragOver = (e: DragEndEvent) => {
    if (e.active.data.current?.type === "row") {
      setDrop(null);
      return;
    }
    const d = computeBlockDrop(e);
    setDrop(d ? { colId: d.colId, index: d.index } : null);
  };

  const onDragEnd = (e: DragEndEvent) => {
    setActiveId(null);
    setDrop(null);
    const { active, over } = e;
    if (!over) return;
    const activeIdStr = String(active.id);

    if (active.data.current?.type === "row") {
      const overId = String(over.id);
      const from = rows.findIndex((r) => `row:${r.id}` === activeIdStr);
      const toRowId = overId.startsWith("row:") ? overId.slice(4) : (over.data.current?.rowId as string | undefined);
      const to = rows.findIndex((r) => r.id === toRowId);
      if (from >= 0 && to >= 0 && from !== to) setRows(arrayMove(rows, from, to));
      return;
    }

    const src = findBlock(activeIdStr);
    const d = computeBlockDrop(e);
    if (!src || !d) return;
    if (src.colId === d.colId && (src.index === d.index || src.index === d.index - 1)) return; // sin cambio real
    moveBlockTo(src, d.rowId, d.colId, d.index);
  };

  // Bloque seleccionado / arrastrado.
  let selBlock: Section | null = null;
  if (sel?.blockId) selBlock = findBlock(sel.blockId)?.block ?? null;
  const activeBlock = activeId && !activeId.startsWith("row:") ? findBlock(activeId)?.block ?? null : null;
  const detected = detectVariables(value, subject);

  return (
    <div className="grid grid-cols-1 gap-3 lg:grid-cols-[200px_minmax(0,1fr)_320px]">
      {/* Paleta + Añadir fila + Tema */}
      <div className="space-y-4 lg:sticky lg:top-4 lg:self-start">
        <div className="rounded-xl border border-border bg-card p-3">
          <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">Elementos</p>
          <p className="mb-2 text-[11px] text-muted-foreground">Se añaden a la columna seleccionada.</p>
          <div className="grid grid-cols-2 gap-2 lg:grid-cols-1">
            {ELEMENTS.map((el) => {
              const Icon = el.icon;
              return (
                <button key={el.type} type="button" onClick={() => addBlock(el.type)} className="flex items-center gap-2 rounded-lg border border-border bg-background px-2.5 py-2 text-sm text-foreground transition-colors hover:border-primary/40 hover:bg-accent">
                  <Icon className="size-4 shrink-0 text-muted-foreground" />
                  <span className="truncate">{el.label}</span>
                  <Plus className="ml-auto size-3.5 shrink-0 text-muted-foreground" />
                </button>
              );
            })}
          </div>
        </div>

        <div className="rounded-xl border border-border bg-card p-3">
          <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">Añadir fila</p>
          <div className="grid grid-cols-1 gap-1.5">
            {LAYOUTS.map((l) => {
              const Icon = l.icon;
              return (
                <button key={l.key} type="button" onClick={() => addRow(l.widths)} className="flex items-center gap-2 rounded-lg border border-border bg-background px-2.5 py-1.5 text-sm text-foreground transition-colors hover:border-primary/40 hover:bg-accent">
                  <Icon className="size-4 shrink-0 text-muted-foreground" />
                  <span className="truncate">{l.label}</span>
                </button>
              );
            })}
          </div>
        </div>

        <div className="rounded-xl border border-border bg-card p-3">
          <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">Tema</p>
          <div className="space-y-3">
            <div className="space-y-1.5">
              <Label htmlFor="th-color">Color primario</Label>
              <div className="flex items-center gap-2">
                <Input id="th-color" type="color" className="h-9 w-12 cursor-pointer p-1" value={theme.primaryColor || "#0ea5e9"} onChange={(e) => setTheme({ primaryColor: e.target.value })} />
                <span className="font-mono text-xs text-muted-foreground">{theme.primaryColor || "#0ea5e9"}</span>
              </div>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="th-font">Tipografía</Label>
              <Select id="th-font" value={theme.fontFamily || ""} onChange={(e) => setTheme({ fontFamily: e.target.value, fontImport: fontImportFor(e.target.value) })}>
                {SYSTEM_FONTS.map((f) => (
                  <option key={f.label} value={f.value}>
                    {f.label}
                  </option>
                ))}
                {WEB_FONTS.map((f) => (
                  <option key={f.label} value={f.value}>
                    {f.label}
                  </option>
                ))}
              </Select>
            </div>
          </div>
        </div>
      </div>

      {/* Lienzo + objeto de envío (columna central) */}
      <div className="min-w-0 space-y-3">
      <div className="rounded-xl border border-border bg-muted/30">
        <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border p-2">
          <div className="inline-flex rounded-lg border border-border bg-card p-0.5">
            {(Object.keys(VP) as (keyof typeof VP)[]).map((k) => {
              const V = VP[k];
              const Icon = V.icon;
              return (
                <button key={k} type="button" onClick={() => setViewport(k)} className={cn("flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium transition-colors", viewport === k ? "bg-primary/10 text-primary" : "text-muted-foreground hover:text-foreground")}>
                  <Icon className="size-3.5" />
                  {V.label}
                </button>
              );
            })}
          </div>
          <div className="inline-flex rounded-lg border border-border bg-card p-0.5">
            <button type="button" onClick={() => setMode("design")} className={cn("flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium transition-colors", mode === "design" ? "bg-primary/10 text-primary" : "text-muted-foreground hover:text-foreground")}>
              <LayoutTemplate className="size-3.5" />
              Diseño
            </button>
            <button type="button" onClick={() => setMode("preview")} className={cn("flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium transition-colors", mode === "preview" ? "bg-primary/10 text-primary" : "text-muted-foreground hover:text-foreground")}>
              <Monitor className="size-3.5" />
              Vista previa
            </button>
          </div>
        </div>

        <div className="overflow-auto p-4">
          <div className="mx-auto" style={{ width: VP[viewport].w, maxWidth: "100%" }}>
            <div className="mb-1.5 text-center text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
              {VP[viewport].label} · {VP[viewport].w}px
            </div>
            <div className="overflow-hidden rounded-xl border border-border bg-white shadow-sm">
              {mode === "preview" ? (
                previewPaused || !preview ? (
                  <div className="p-8 text-center text-sm text-muted-foreground">{previewPaused ? "Vista previa en pausa (variable sin cerrar)…" : "Renderizando…"}</div>
                ) : (
                  <iframe title="preview" sandbox="" className="h-[560px] w-full bg-white" srcDoc={preview.html} />
                )
              ) : rows.length === 0 ? (
                <div className="flex flex-col items-center justify-center gap-2 p-10 text-center text-muted-foreground">
                  <Rows3 className="size-6" />
                  <p className="text-sm">Añade una fila desde la izquierda y luego coloca elementos en sus columnas.</p>
                </div>
              ) : (
                <DndContext sensors={sensors} collisionDetection={closestCorners} onDragStart={(e: DragStartEvent) => setActiveId(String(e.active.id))} onDragOver={onDragOver} onDragEnd={onDragEnd} onDragCancel={() => { setActiveId(null); setDrop(null); }}>
                  <SortableContext items={rows.map((r) => `row:${r.id}`)} strategy={verticalListSortingStrategy}>
                    <div style={{ fontFamily: theme.fontFamily || "Arial, Helvetica, sans-serif" }}>
                      {rows.map((r) => (
                        <RowView
                          key={r.id}
                          row={r}
                          theme={theme}
                          sel={sel}
                          drop={drop}
                          onSelectCol={(colId) => setSel({ rowId: r.id, colId })}
                          onSelectBlock={(colId, blockId) => setSel({ rowId: r.id, colId, blockId })}
                          onLayout={(w) => setRowLayout(r.id, w)}
                          onRemove={() => removeRow(r.id)}
                          onBlockDup={(colId, id) => duplicateBlock(r.id, colId, id)}
                          onBlockRemove={(colId, id) => removeBlock(r.id, colId, id)}
                        />
                      ))}
                    </div>
                  </SortableContext>
                  <DragOverlay>
                    {activeBlock ? (
                      <div className="overflow-hidden rounded-md border border-primary bg-white opacity-90 shadow-lg">
                        <BlockView section={activeBlock} theme={theme} />
                      </div>
                    ) : activeId?.startsWith("row:") ? (
                      <div className="rounded-md border border-primary bg-card px-3 py-2 text-xs font-medium text-primary shadow-lg">Moviendo fila…</div>
                    ) : null}
                  </DragOverlay>
                </DndContext>
              )}
            </div>
          </div>
        </div>
      </div>
      {footer}
      </div>

      {/* Inspector */}
      <div className="space-y-4 lg:sticky lg:top-4 lg:self-start">
        <div className="rounded-xl border border-border bg-card p-3">
          <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">{selBlock ? `Bloque · ${ELEMENT_LABEL[selBlock.type] ?? selBlock.type}` : "Inspector"}</p>
          {!selBlock || !sel ? (
            <p className="text-sm text-muted-foreground">Selecciona un bloque del lienzo para editar su contenido y estilo. Para añadir, elige una columna y pulsa un elemento.</p>
          ) : (
            <div className="space-y-3">
              {(FIELDS[selBlock.type] ?? []).filter((f) => !f.showIf || f.showIf(selBlock!.props)).map((f) => (
                <div key={f.key} className="space-y-1.5">
                  <Label htmlFor={`f-${f.key}`}>{f.label}</Label>
                  {f.options ? (
                    <Select id={`f-${f.key}`} value={selBlock.props[f.key] ?? f.options[0].value} onChange={(e) => setProp(sel.rowId, sel.colId, selBlock!.id, f.key, e.target.value)}>
                      {f.options.map((o) => (
                        <option key={o.value} value={o.value}>
                          {o.label}
                        </option>
                      ))}
                    </Select>
                  ) : f.number ? (
                    <Input id={`f-${f.key}`} type="number" placeholder={f.placeholder} value={selBlock.props[f.key] ?? ""} onChange={(e) => setProp(sel.rowId, sel.colId, selBlock!.id, f.key, e.target.value)} />
                  ) : f.variable ? (
                    <VariableField id={`f-${f.key}`} multiline={f.textarea} className={f.textarea ? "h-24" : undefined} placeholder={f.placeholder} value={selBlock.props[f.key] ?? ""} used={detected} onChange={(v) => setProp(sel.rowId, sel.colId, selBlock!.id, f.key, v)} />
                  ) : (
                    <Input id={`f-${f.key}`} placeholder={f.placeholder} value={selBlock.props[f.key] ?? ""} onChange={(e) => setProp(sel.rowId, sel.colId, selBlock!.id, f.key, e.target.value)} />
                  )}
                </div>
              ))}

              {(STYLE[selBlock.type]?.align || STYLE[selBlock.type]?.fontSize || STYLE[selBlock.type]?.color) && (
                <div className="space-y-2 border-t border-border pt-3">
                  <p className="text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">Estilo</p>
                  {STYLE[selBlock.type]?.align && (
                    <div>
                      <Label className="mb-2 block">Alineación</Label>
                      <div className="inline-flex rounded-md border border-border bg-background p-0.5">
                        {ALIGNS.map((a) => {
                          const Icon = a.icon;
                          const active = (selBlock!.props.align || (selBlock!.type === "text" ? "left" : "center")) === a.value;
                          return (
                            <button key={a.value} type="button" title={a.label} onClick={() => setProp(sel.rowId, sel.colId, selBlock!.id, "align", a.value)} className={cn("flex size-7 items-center justify-center rounded transition-colors", active ? "bg-primary/10 text-primary" : "text-muted-foreground hover:text-foreground")}>
                              <Icon className="size-4" />
                            </button>
                          );
                        })}
                      </div>
                    </div>
                  )}
                  <div className="flex gap-2">
                    {STYLE[selBlock.type]?.fontSize && (
                      <div className="flex-1 space-y-1">
                        <Label htmlFor="st-size">Tamaño (px)</Label>
                        <Input id="st-size" type="number" min={8} max={96} placeholder="15" value={selBlock.props.fontSize ?? ""} onChange={(e) => setProp(sel.rowId, sel.colId, selBlock!.id, "fontSize", e.target.value)} />
                      </div>
                    )}
                    {STYLE[selBlock.type]?.color && (
                      <div className="space-y-1">
                        <Label htmlFor="st-color">Color</Label>
                        <Input id="st-color" type="color" className="h-9 w-12 cursor-pointer p-1" value={selBlock.props.color || "#333333"} onChange={(e) => setProp(sel.rowId, sel.colId, selBlock!.id, "color", e.target.value)} />
                      </div>
                    )}
                  </div>
                </div>
              )}
            </div>
          )}
        </div>

        <VariablesPanel detected={detected} specs={variables} onChange={onVariablesChange} testValue={testValue} onTestChange={onTestChange} />
      </div>
    </div>
  );
}

// RowView: fila arrastrable (handle) con su barra y sus columnas.
function RowView({
  row,
  theme,
  sel,
  drop,
  onSelectCol,
  onSelectBlock,
  onLayout,
  onRemove,
  onBlockDup,
  onBlockRemove,
}: {
  row: Row;
  theme: NonNullable<Structure["theme"]>;
  sel: Sel;
  drop: { colId: string; index: number } | null;
  onSelectCol: (colId: string) => void;
  onSelectBlock: (colId: string, blockId: string) => void;
  onLayout: (widths: number[]) => void;
  onRemove: () => void;
  onBlockDup: (colId: string, id: string) => void;
  onBlockRemove: (colId: string, id: string) => void;
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: `row:${row.id}`, data: { type: "row", rowId: row.id } });
  const style = { transform: CSS.Transform.toString(transform), transition, opacity: isDragging ? 0.5 : 1 };
  const total = row.columns.reduce((s, c) => s + (c.width > 0 ? c.width : 1), 0);
  const [layoutOpen, setLayoutOpen] = useState(false);
  return (
    <div ref={setNodeRef} style={style} className="group/row relative border-b border-dashed border-border">
      {/* Riel de FILA (izquierda): arrastrar · layout · eliminar */}
      <div className={cn("absolute left-1 top-1 z-30 flex-col items-center gap-0.5 rounded-md border border-border bg-card p-0.5 shadow-sm", layoutOpen ? "flex" : "hidden group-hover/row:flex")}>
        <button type="button" {...attributes} {...listeners} title="Arrastrar fila" className="flex size-6 cursor-grab items-center justify-center rounded text-muted-foreground hover:bg-accent hover:text-foreground">
          <GripVertical className="size-3.5" />
        </button>
        <Popover open={layoutOpen} onOpenChange={setLayoutOpen}>
          <PopoverTrigger asChild>
            <button type="button" title="Layout de columnas" className="flex size-6 items-center justify-center rounded text-muted-foreground hover:bg-accent hover:text-foreground">
              <Columns2 className="size-3.5" />
            </button>
          </PopoverTrigger>
          <PopoverContent align="start" side="right" sideOffset={6} onOpenAutoFocus={(e) => e.preventDefault()} className="w-52 p-2">
            <p className="mb-1 text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">Columnas</p>
            <div className="grid gap-1">
              {LAYOUTS.map((l) => {
                const Icon = l.icon;
                const active = row.columns.length === l.widths.length && row.columns.every((c, i) => (c.width > 0 ? c.width : 1) === l.widths[i]);
                return (
                  <button
                    key={l.key}
                    type="button"
                    onClick={() => {
                      onLayout(l.widths);
                      setLayoutOpen(false);
                    }}
                    className={cn("flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors hover:bg-accent", active ? "bg-primary/10 text-primary" : "text-foreground")}
                  >
                    <Icon className="size-4 shrink-0" />
                    <span className="flex-1 text-left">{l.label}</span>
                    {active && <Check className="size-3.5" />}
                  </button>
                );
              })}
            </div>
            <div className="mt-2 border-t border-border pt-2">
              <button
                type="button"
                onClick={() => {
                  onRemove();
                  setLayoutOpen(false);
                }}
                className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm text-destructive transition-colors hover:bg-destructive/10"
              >
                <Trash2 className="size-4 shrink-0" />
                Eliminar fila
              </button>
            </div>
          </PopoverContent>
        </Popover>
      </div>

      <div className="flex">
        {row.columns.map((c) => {
          const pct = (100 * (c.width > 0 ? c.width : 1)) / total;
          return (
            <ColumnView
              key={c.id}
              column={c}
              theme={theme}
              width={pct}
              selectedCol={sel?.colId === c.id}
              selBlockId={sel?.blockId}
              dropIndex={drop?.colId === c.id ? drop.index : null}
              onSelectCol={() => onSelectCol(c.id)}
              onSelectBlock={(blockId) => onSelectBlock(c.id, blockId)}
              onBlockDup={(id) => onBlockDup(c.id, id)}
              onBlockRemove={(id) => onBlockRemove(c.id, id)}
            />
          );
        })}
      </div>
    </div>
  );
}

// ColumnView: zona soltable (droppable) con sus bloques ordenables.
function ColumnView({
  column,
  theme,
  width,
  selectedCol,
  selBlockId,
  dropIndex,
  onSelectCol,
  onSelectBlock,
  onBlockDup,
  onBlockRemove,
}: {
  column: Column;
  theme: NonNullable<Structure["theme"]>;
  width: number;
  selectedCol: boolean;
  selBlockId?: string;
  dropIndex: number | null;
  onSelectCol: () => void;
  onSelectBlock: (blockId: string) => void;
  onBlockDup: (id: string) => void;
  onBlockRemove: (id: string) => void;
}) {
  const { setNodeRef, isOver } = useDroppable({ id: `col:${column.id}`, data: { type: "col", colId: column.id } });
  const empty = column.blocks.length === 0;
  return (
    <div
      ref={setNodeRef}
      onClick={(e) => {
        e.stopPropagation();
        onSelectCol();
      }}
      style={{ width: `${width}%` }}
      className={cn("min-h-[56px] border border-transparent", selectedCol ? "bg-primary/5 outline outline-2 -outline-offset-2 outline-primary/60" : isOver ? "bg-primary/10" : "hover:bg-muted/40")}
    >
      <SortableContext items={column.blocks.map((b) => b.id)} strategy={verticalListSortingStrategy}>
        {empty ? (
          <div className={cn("flex h-full min-h-[56px] items-center justify-center p-3 text-center text-[11px]", dropIndex !== null ? "bg-primary/10 text-primary" : "text-muted-foreground")}>
            {dropIndex !== null ? "Soltar aquí" : selectedCol ? "Columna vacía · pulsa un elemento o suelta aquí" : "Columna vacía"}
          </div>
        ) : (
          column.blocks.map((b, i) => (
            <div key={b.id}>
              {dropIndex === i && <DropLine />}
              <BlockRow block={b} theme={theme} selected={selBlockId === b.id} onSelect={() => onSelectBlock(b.id)} onDup={() => onBlockDup(b.id)} onRemove={() => onBlockRemove(b.id)} />
              {dropIndex === i + 1 && i === column.blocks.length - 1 && <DropLine />}
            </div>
          ))
        )}
      </SortableContext>
    </div>
  );
}

// DropLine: guía visual del punto de inserción durante el arrastre.
function DropLine() {
  return (
    <div className="relative h-0.5">
      <div className="absolute inset-x-2 top-0 h-0.5 rounded-full bg-primary" />
      <div className="absolute left-1.5 top-1/2 size-1.5 -translate-y-1/2 rounded-full bg-primary" />
    </div>
  );
}

// BlockRow: bloque arrastrable (handle) con su barra de acciones.
function BlockRow({ block, theme, selected, onSelect, onDup, onRemove }: { block: Section; theme: NonNullable<Structure["theme"]>; selected: boolean; onSelect: () => void; onDup: () => void; onRemove: () => void }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: block.id, data: { type: "block" } });
  const style = { transform: CSS.Transform.toString(transform), transition, opacity: isDragging ? 0.4 : 1 };
  const Icon = ELEMENT_ICON[block.type] ?? Type;
  return (
    <div
      ref={setNodeRef}
      style={style}
      onClick={(e) => {
        e.stopPropagation();
        onSelect();
      }}
      className={cn("group/blk relative cursor-pointer", selected ? "outline outline-2 -outline-offset-2 outline-primary" : "hover:outline hover:outline-1 hover:-outline-offset-1 hover:outline-primary/30")}
    >
      {/* Barra de BLOQUE (derecha): etiqueta · arrastrar · duplicar · eliminar */}
      <div className={cn("absolute right-1 top-1 z-20 items-center gap-0.5 rounded-md border border-border bg-card p-0.5 pl-1.5 shadow-sm", selected ? "flex" : "hidden group-hover/blk:flex")}>
        <span className="flex items-center gap-1 text-[10px] font-medium uppercase text-primary">
          <Icon className="size-3" />
          {ELEMENT_LABEL[block.type] ?? block.type}
        </span>
        <span className="mx-0.5 h-4 w-px bg-border" />
        <button type="button" {...attributes} {...listeners} onClick={(e) => e.stopPropagation()} title="Arrastrar" className="flex size-6 cursor-grab items-center justify-center rounded text-muted-foreground hover:bg-accent hover:text-foreground">
          <GripVertical className="size-3.5" />
        </button>
        <MiniBtn label="Duplicar" onClick={onDup}>
          <Copy className="size-3.5" />
        </MiniBtn>
        <MiniBtn label="Eliminar" danger onClick={onRemove}>
          <Trash2 className="size-3.5" />
        </MiniBtn>
      </div>
      <BlockView section={block} theme={theme} />
    </div>
  );
}

function MiniBtn({ label, onClick, danger, children }: { label: string; onClick: () => void; danger?: boolean; children: React.ReactNode }) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      onClick={(e) => {
        e.stopPropagation();
        onClick();
      }}
      className={cn("flex size-6 items-center justify-center rounded text-muted-foreground hover:bg-accent hover:text-foreground", danger && "hover:bg-destructive/10 hover:text-destructive")}
    >
      {children}
    </button>
  );
}

// BlockView: representación visual aproximada (WYSIWYG). Aplica alineación,
// tamaño y color. Las variables se muestran tal cual ({{x}}).
function BlockView({ section, theme }: { section: Section; theme: NonNullable<Structure["theme"]> }) {
  const p = (k: string) => section.props[k] ?? "";
  const primary = theme.primaryColor || "#0ea5e9";
  const align = (def: string) => (["left", "center", "right", "justify"].includes(p("align")) ? p("align") : def) as "left" | "center" | "right" | "justify";
  const fs = (def: number) => {
    const n = Number(p("fontSize"));
    return n >= 8 && n <= 96 ? n : def;
  };
  const color = (def: string) => (/^#[0-9a-fA-F]{3,8}$/.test(p("color")) ? p("color") : def);

  switch (section.type) {
    case "header":
      return (
        <div className="px-6 pb-2 pt-6" style={{ textAlign: align("center") }}>
          {p("logoUrl") ? <img src={p("logoUrl")} alt="" className="mb-3 inline-block h-10 object-contain" /> : null}
          <h1 className="font-semibold" style={{ fontSize: fs(22), color: color("#111111") }}>
            {p("title") || "Título"}
          </h1>
        </div>
      );
    case "text":
      return (
        <p className="whitespace-pre-wrap px-6 py-2 leading-relaxed" style={{ textAlign: align("left"), fontSize: fs(15), color: color("#333333") }}>
          {p("text") || "Texto…"}
        </p>
      );
    case "button":
      return (
        <div className="px-6 py-4" style={{ textAlign: align("center") }}>
          <span className="inline-block rounded-md px-5 py-3 text-[15px] font-medium text-white" style={{ background: primary }}>
            {p("text") || "Botón"}
          </span>
        </div>
      );
    case "image":
      return (
        <div className="px-6 py-2" style={{ textAlign: align("center") }}>
          {p("url") ? (
            <img src={p("url")} alt={p("alt")} className="inline-block max-w-full" />
          ) : (
            <div className="flex h-32 flex-col items-center justify-center gap-2 rounded-lg border-2 border-dashed border-neutral-300 bg-neutral-50 text-neutral-400">
              <span className="flex size-10 items-center justify-center rounded-full bg-neutral-200/70 text-neutral-500">
                <ImageIcon className="size-5" />
              </span>
              <span className="text-xs font-medium">Imagen · añade una URL</span>
            </div>
          )}
        </div>
      );
    case "divider": {
      const thickness = Math.min(40, Math.max(1, Number(p("thickness")) || 1));
      if (p("orientation") === "vertical") {
        const h = Math.min(600, Math.max(4, Number(p("height")) || 40));
        return (
          <div className="px-6 py-2 text-center">
            <span style={{ display: "inline-block", width: thickness, height: h, background: "#e5e5e5" }} />
          </div>
        );
      }
      return (
        <div className="px-6 py-2">
          <div style={{ borderTop: `${thickness}px solid #e5e5e5` }} />
        </div>
      );
    }
    case "spacer":
      return <div style={{ height: Number(p("height")) || 16 }} className="bg-[repeating-linear-gradient(45deg,transparent,transparent_6px,rgba(0,0,0,0.03)_6px,rgba(0,0,0,0.03)_12px)]" />;
    case "footer":
      return (
        <p className="whitespace-pre-wrap px-6 pb-6 pt-2" style={{ textAlign: align("center"), fontSize: fs(12), color: color("#999999") }}>
          {p("text") || "Pie…"}
        </p>
      );
    default:
      return null;
  }
}
