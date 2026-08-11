import { Fragment, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Bell, Code2, Eye, FileText, Inbox, Languages, LayoutTemplate, Mail, MessageSquare, Pencil, Plus, Smartphone, type LucideIcon } from "lucide-react";
import { del, get, post } from "../../api/client";
import { Badge, Card, ConfirmButton, ErrorText, useAsync } from "../../ui";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { cn } from "@/lib/utils";
import { detectVariables, detectVariablesInText, type Preview } from "./TemplateBuilder";
import { sampleValue, toStructure } from "../templateEditor.utils";

interface Template {
  id: string;
  key: string;
  channel: string;
  locale: string;
  version: number;
  name: string;
  kind?: string;
}
// La plantilla completa (GET por id) trae lo necesario para renderizar el preview.
interface TemplateFull extends Template {
  subject?: string;
  structure?: unknown;
  body?: string;
}

// EditorBadge indica cómo se edita la plantilla: por bloques o en HTML avanzado.
function EditorBadge({ kind }: { kind?: string }) {
  const html = kind === "html";
  const Icon = html ? Code2 : LayoutTemplate;
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium",
        html ? "border-primary/30 bg-primary/5 text-primary" : "border-border bg-muted text-muted-foreground",
      )}
    >
      <Icon className="size-3" />
      {html ? "HTML" : "Visual"}
    </span>
  );
}

// Metadatos por canal: etiqueta, icono y descripción corta para el selector.
const CHANNEL_META: { key: string; label: string; icon: LucideIcon; hint: string }[] = [
  { key: "EMAIL", label: "Email", icon: Mail, hint: "Correo con asunto y cuerpo HTML." },
  { key: "PUSH", label: "Push", icon: Bell, hint: "Notificación push al dispositivo." },
  { key: "SMS", label: "SMS", icon: MessageSquare, hint: "Mensaje de texto corto." },
  { key: "IN_APP", label: "In-app", icon: Inbox, hint: "Aviso dentro de la aplicación." },
];
const CHANNEL_LABEL: Record<string, string> = Object.fromEntries(CHANNEL_META.map((c) => [c.key, c.label]));

// Una "familia" agrupa las variantes de idioma que comparten key: es lo que se
// asocia a un tipo de notificación (el envío elige la variante por locale).
interface Family {
  key: string;
  name: string;
  variants: Template[];
}

function groupByKey(templates: Template[]): Family[] {
  const byKey = new Map<string, Family>();
  for (const t of templates) {
    const f = byKey.get(t.key) ?? { key: t.key, name: t.name, variants: [] };
    f.variants.push(t);
    byKey.set(t.key, f);
  }
  for (const f of byKey.values()) f.variants.sort((a, b) => a.locale.localeCompare(b.locale));
  return Array.from(byKey.values());
}

export default function Templates({ appId }: { appId: string }) {
  const navigate = useNavigate();
  const base = `/admin/applications/${appId}`;
  const list = useAsync(() => get<{ data: Template[] }>(`${base}/templates`), [appId]);
  const [channel, setChannel] = useState<string>("EMAIL");
  const [actErr, setActErr] = useState<unknown>(null);

  const all = list.data?.data ?? [];
  const templates = all.filter((t) => t.channel === channel);
  const families = groupByKey(templates);
  const countFor = (c: string) => all.filter((t) => t.channel === c).length;

  const goNew = () => navigate(`/apps/${appId}/templates/new?channel=${channel}`);
  const goEdit = (id: string) => navigate(`/apps/${appId}/templates/${id}/edit`);
  // Crear una variante de idioma: abre el alta CLONANDO el contenido de esta
  // plantilla (misma key, idioma distinto) para solo traducir los textos.
  const goNewLang = (t: Template) => navigate(`/apps/${appId}/templates/new?channel=${t.channel}&from=${t.id}`);

  // Vista previa desde el listado: renderiza con datos de ejemplo, sin abrir el editor.
  const [previewOf, setPreviewOf] = useState<Template | null>(null);
  const [preview, setPreview] = useState<Preview | null>(null);
  const [previewErr, setPreviewErr] = useState<unknown>(null);
  async function openPreview(t: Template) {
    setPreviewOf(t);
    setPreview(null);
    setPreviewErr(null);
    try {
      const full = await get<TemplateFull>(`${base}/templates/${t.id}`);
      const htmlMode = full.channel === "EMAIL" && full.kind === "html";
      const detected = htmlMode
        ? detectVariablesInText(`${full.body ?? ""} ${full.subject ?? ""}`)
        : detectVariables(toStructure(full.structure, full.channel), full.subject ?? "");
      const payload = Object.fromEntries(detected.map((v) => [v, sampleValue(v)]));
      const reqBody = htmlMode
        ? { channel: full.channel, kind: "html", subject: full.subject, body: full.body, payload }
        : { channel: full.channel, subject: full.subject, structure: full.structure, payload };
      setPreview(await post<Preview>(`${base}/templates/preview`, reqBody));
    } catch (err) {
      setPreviewErr(err);
    }
  }
  const remove = (t: Template) =>
    del(`${base}/templates/${t.id}`)
      .then(() => list.reload())
      .catch(setActErr);

  return (
    <Card
      title="Plantillas"
      actions={
        <Button onClick={goNew} className="max-md:w-9 max-md:px-0">
          <Plus />
          <span className="sr-only md:not-sr-only">Nueva</span>
        </Button>
      }
    >
      <p className="-mt-1 mb-3 text-sm text-muted-foreground">
        Diseños reutilizables del mensaje por canal, con variables {`{{…}}`} que se rellenan en cada envío. Los idiomas de una misma plantilla comparten key: el tipo de
        notificación asocia la plantilla y el envío elige el idioma según el «locale» del mensaje.
      </p>

      {/* Selector de canal: cada canal es un botón con su icono y nº de plantillas. */}
      <p className="mb-1.5 text-xs font-medium uppercase tracking-wide text-muted-foreground">Canal</p>
      <Tabs value={channel} onValueChange={setChannel} className="mb-4">
        <TabsList className="grid h-auto w-full grid-cols-2 gap-2 bg-transparent p-0 sm:grid-cols-4">
          {CHANNEL_META.map((c) => {
            const Icon = c.icon;
            const count = countFor(c.key);
            return (
              <TabsTrigger
                key={c.key}
                value={c.key}
                className={cn(
                  "group flex h-auto flex-col items-start gap-1.5 whitespace-normal rounded-lg border border-border bg-card px-3 py-2.5 text-left shadow-none transition-all",
                  "hover:border-primary/40 hover:bg-muted/40",
                  "data-[state=active]:border-primary data-[state=active]:bg-primary/5 data-[state=active]:shadow-sm",
                )}
              >
                <div className="flex w-full items-center gap-2">
                  <Icon className="size-4 shrink-0 text-muted-foreground transition-colors group-data-[state=active]:text-primary" />
                  <span className="text-sm font-semibold text-foreground">{c.label}</span>
                  <span
                    className={cn(
                      "ml-auto rounded-full px-1.5 text-xs tabular-nums",
                      "bg-muted text-muted-foreground group-data-[state=active]:bg-primary/10 group-data-[state=active]:text-primary",
                    )}
                  >
                    {count}
                  </span>
                </div>
                <span className="text-xs font-normal text-muted-foreground max-sm:hidden">{c.hint}</span>
              </TabsTrigger>
            );
          })}
        </TabsList>
      </Tabs>

      <ErrorText error={list.error} />
      <ErrorText error={actErr} />

      {list.data && templates.length === 0 && (
        <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed border-border py-10 text-center">
          <span className="flex h-12 w-12 items-center justify-center rounded-full bg-muted text-muted-foreground">
            <FileText className="size-5" />
          </span>
          <div className="space-y-1">
            <p className="text-sm font-medium text-foreground">Aún no hay plantillas de {CHANNEL_LABEL[channel]}</p>
            <p className="text-sm text-muted-foreground">Crea la primera con el editor.</p>
          </div>
          <Button onClick={goNew}>
            <Plus />
            Nueva
          </Button>
        </div>
      )}

      {/* Tarjetas: < 768px — una tarjeta por familia (key), una fila por idioma. */}
      {templates.length > 0 && (
        <div className="space-y-2 md:hidden">
          {families.map((f) => (
            <div key={f.key} className="border border-border bg-card p-3">
              <div className="flex items-start justify-between gap-2">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium text-foreground">{f.name}</p>
                  <p className="mt-0.5 truncate text-xs text-muted-foreground">{f.key}</p>
                </div>
                <Button variant="outline" size="sm" onClick={() => goNewLang(f.variants[0])} title="Crear esta plantilla en otro idioma">
                  <Languages />
                  Añadir idioma
                </Button>
              </div>
              <div className="mt-2 divide-y divide-border/60 border-t border-border/60">
                {f.variants.map((t) => (
                  <div key={t.id} className="flex min-w-0 items-center gap-2 py-1.5">
                    <Badge value={t.locale} />
                    <span className="bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">v{t.version}</span>
                    <EditorBadge kind={t.kind} />
                    <div className="ml-auto flex items-center gap-1">
                      <Button variant="outline" size="sm" onClick={() => goEdit(t.id)} className="w-8 px-0" title="Editar">
                        <Pencil />
                        <span className="sr-only">Editar</span>
                      </Button>
                      <Button variant="outline" size="sm" onClick={() => openPreview(t)} className="w-8 px-0" title="Vista previa">
                        <Eye />
                        <span className="sr-only">Vista previa</span>
                      </Button>
                      <ConfirmButton message={`¿Eliminar la plantilla «${t.name}» (${t.locale})?`} onConfirm={() => remove(t)}>
                        Eliminar
                      </ConfirmButton>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Tabla: ≥ 768px — cabecera por familia (key) y una fila por idioma. */}
      {templates.length > 0 && (
        <div className="hidden overflow-x-auto md:block">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-border text-xs uppercase tracking-wide text-muted-foreground">
                <th className="py-2 pr-4 font-medium">Idioma</th>
                <th className="py-2 pr-4 font-medium">Versión</th>
                <th className="py-2 pr-4 font-medium">Editor</th>
                <th className="py-2 font-medium"></th>
              </tr>
            </thead>
            <tbody>
              {families.map((f) => (
                <Fragment key={f.key}>
                  <tr className="border-b border-border bg-muted/30">
                    <td colSpan={4} className="px-2 py-2">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-medium text-foreground">{f.name}</span>
                        <Badge value={f.key} />
                        <span className="text-xs text-muted-foreground">
                          {f.variants.length} {f.variants.length === 1 ? "idioma" : "idiomas"}
                        </span>
                        <div className="ml-auto">
                          <Button variant="outline" size="sm" onClick={() => goNewLang(f.variants[0])} title="Crear esta plantilla en otro idioma">
                            <Languages />
                            Añadir idioma
                          </Button>
                        </div>
                      </div>
                    </td>
                  </tr>
                  {f.variants.map((t) => (
                    <tr key={t.id} className="border-b border-border/60 align-top">
                      <td className="py-2 pl-2 pr-4">
                        <div className="flex items-center gap-2">
                          <Badge value={t.locale} />
                          {t.name !== f.name && <span className="truncate text-xs text-muted-foreground">{t.name}</span>}
                        </div>
                      </td>
                      <td className="py-2 pr-4">
                        <span className="bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">v{t.version}</span>
                      </td>
                      <td className="py-2 pr-4">
                        <EditorBadge kind={t.kind} />
                      </td>
                      <td className="py-2">
                        <div className="flex items-center justify-end gap-1">
                          <Button variant="outline" size="sm" onClick={() => goEdit(t.id)} className="max-lg:w-8 max-lg:px-0">
                            <Pencil />
                            <span className="sr-only lg:not-sr-only">Editar</span>
                          </Button>
                          <Button variant="outline" size="sm" onClick={() => openPreview(t)} className="max-lg:w-8 max-lg:px-0">
                            <Eye />
                            <span className="sr-only lg:not-sr-only">Vista previa</span>
                          </Button>
                          <ConfirmButton message={`¿Eliminar la plantilla «${t.name}» (${t.locale})?`} onConfirm={() => remove(t)}>
                            Eliminar
                          </ConfirmButton>
                        </div>
                      </td>
                    </tr>
                  ))}
                </Fragment>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Vista previa rápida (modal): renderiza la plantilla con datos de ejemplo. */}
      <Dialog
        open={!!previewOf}
        onOpenChange={(o) => {
          if (!o) {
            setPreviewOf(null);
            setPreview(null);
            setPreviewErr(null);
          }
        }}
      >
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>Vista previa · {previewOf?.name}</DialogTitle>
            <DialogDescription>Renderizada con datos de ejemplo.</DialogDescription>
          </DialogHeader>
          <ErrorText error={previewErr} />
          {!preview && !previewErr && <p className="text-sm text-muted-foreground">Renderizando…</p>}
          {preview && (
            <div className="space-y-2">
              {preview.missingVariables && preview.missingVariables.length > 0 && (
                <p className="rounded-md border border-border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
                  Faltan variables: {preview.missingVariables.join(", ")}
                </p>
              )}
              {previewOf?.channel === "EMAIL" ? (
                <iframe title="preview" sandbox="" className="h-[26rem] w-full rounded-lg border border-border bg-white" srcDoc={preview.html} />
              ) : previewOf?.channel === "SMS" ? (
                <div className="mx-auto max-w-sm">
                  <div className="flex items-start gap-2">
                    <span className="mt-1 flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground">
                      <MessageSquare className="size-4" />
                    </span>
                    <div className="whitespace-pre-wrap rounded-2xl rounded-tl-sm bg-muted px-3 py-2 text-sm text-foreground">
                      {preview.html || <span className="text-muted-foreground">—</span>}
                    </div>
                  </div>
                </div>
              ) : (
                <div className="mx-auto max-w-sm rounded-xl border border-border bg-card p-3 shadow-sm">
                  <div className="flex items-start gap-3">
                    <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                      {previewOf?.channel === "PUSH" ? <Smartphone className="size-4" /> : <Inbox className="size-4" />}
                    </span>
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-semibold text-foreground">{preview.subject || "Título"}</p>
                      <p className="mt-0.5 whitespace-pre-wrap text-sm text-muted-foreground">{preview.html || "—"}</p>
                    </div>
                  </div>
                </div>
              )}
            </div>
          )}
        </DialogContent>
      </Dialog>
    </Card>
  );
}
