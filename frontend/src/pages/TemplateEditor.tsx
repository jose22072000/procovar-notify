import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { ArrowLeft, Bell, Check, Code2, Copy, Inbox, LayoutTemplate, Loader2, MessageSquare, Save, Smartphone } from "lucide-react";
import { ApiError, get, post, patch } from "../api/client";
import { Card, ErrorText, Input, Label, Select } from "../ui";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Breadcrumb } from "../components/Breadcrumb";
import { VariableField } from "../components/VariableField";
import { VariablesPanel } from "../components/VariablesPanel";
import { MissingVarsNotice, PreviewPausedNotice } from "../components/PreviewNotices";
import EmailBuilder from "./tabs/EmailBuilder";
import HtmlBuilder from "./tabs/HtmlBuilder";
import { detectVariables, detectVariablesInText, type Preview, type Structure, type VariableSpec } from "./tabs/TemplateBuilder";
import { DEFAULT_HTML, actionSection, buildTemplateBody, defaultStructure, filterBasesForMode, formatHtml, resolveBaseChoice, sampleValue, specsFromSchema, structureHasBlocks, textSection, toStructure } from "./templateEditor.utils";

type EditorMode = "visual" | "html";

interface TemplateDTO {
  id: string;
  key: string;
  channel: string;
  locale: string;
  version: number;
  name: string;
  subject?: string;
  structure?: unknown;
  requiredVariables?: unknown;
  kind?: string;
  body?: string;
}
interface Base {
  id: string;
  key: string;
  name: string;
  channel: string;
  kind?: string;
  body?: string;
  subject?: string;
}

const CHANNEL_LABEL: Record<string, string> = { EMAIL: "Email", PUSH: "Push", SMS: "SMS", IN_APP: "In-app" };
// Sufijo por canal para el placeholder de la Key (welcome_email, welcome_push…).
const KEY_SUFFIX: Record<string, string> = { EMAIL: "email", PUSH: "push", SMS: "sms", IN_APP: "inapp" };
// Idiomas ofrecidos al crear. El backend acepta cualquier código; esta lista es
// solo comodidad del panel — amplíala si necesitas más.
const LOCALES: { code: string; label: string }[] = [
  { code: "en", label: "English (en)" },
  { code: "es", label: "Español (es)" },
  { code: "fr", label: "Français (fr)" },
  { code: "de", label: "Deutsch (de)" },
  { code: "it", label: "Italiano (it)" },
  { code: "pt", label: "Português (pt)" },
];
// pickTargetLocale elige un idioma distinto al de origen para la variante nueva.
function pickTargetLocale(source: string): string {
  return LOCALES.find((l) => l.code !== source)?.code ?? source;
}



function Field({
  label,
  htmlFor,
  required,
  hint,
  error,
  children,
}: {
  label: string;
  htmlFor: string;
  required?: boolean;
  hint?: string;
  error?: string;
  children: ReactNode;
}) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={htmlFor}>
        {label}
        {required && <span className="text-destructive"> *</span>}
      </Label>
      {children}
      {error ? <p className="text-xs font-medium text-destructive">{error}</p> : hint ? <p className="text-xs text-muted-foreground">{hint}</p> : null}
    </div>
  );
}

// ModeSwitch: conmuta el editor visual (bloques) ↔ HTML avanzado. La opción
// visual se deshabilita cuando no hay bloques que recuperar (plantilla html nativa).
function ModeSwitch({ mode, onChange, canRevert }: { mode: EditorMode; onChange: (m: EditorMode) => void; canRevert: boolean }) {
  const visualDisabled = mode === "html" && !canRevert;
  return (
    <Tabs value={mode} onValueChange={(v) => onChange(v as EditorMode)}>
      <TabsList>
        <TabsTrigger value="visual" disabled={visualDisabled} className="gap-1.5">
          <LayoutTemplate className="size-4" />
          Editor visual
        </TabsTrigger>
        <TabsTrigger value="html" className="gap-1.5">
          <Code2 className="size-4" />
          HTML avanzado
        </TabsTrigger>
      </TabsList>
    </Tabs>
  );
}

export default function TemplateEditor() {
  const { appId = "", templateId } = useParams();
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const base = `/admin/applications/${appId}`;
  const editing = !!templateId;
  // fromId: crear una VARIANTE precargando el contenido de otra plantilla (misma
  // key, idioma distinto). Es un alta (no editing), pero arranca con datos.
  const fromId = params.get("from");
  const cloneLang = !editing && !!fromId;

  const [channel, setChannel] = useState(params.get("channel") || "EMAIL");
  const [editorMode, setEditorMode] = useState<EditorMode>("visual");
  const [selectedBase, setSelectedBase] = useState(""); // valor del desplegable de bases
  const [revertConfirmOpen, setRevertConfirmOpen] = useState(false);
  const [form, setForm] = useState(() => ({
    key: params.get("key") || "", // prefill al "añadir idioma" a una key existente
    name: "",
    subject: "",
    locale: params.get("locale") || "en",
    baseTemplateId: "",
    structure: defaultStructure(params.get("channel") || "EMAIL"),
    variables: {} as Record<string, VariableSpec>,
    body: "", // HTML crudo (modo avanzado)
  }));
  const [loading, setLoading] = useState(editing || cloneLang);
  const [saving, setSaving] = useState(false);
  const [loadErr, setLoadErr] = useState<unknown>(null);
  const [saveErr, setSaveErr] = useState<unknown>(null);
  const [submitAttempted, setSubmitAttempted] = useState(false);
  const [touched, setTouched] = useState<Record<string, boolean>>({});

  const [bases, setBases] = useState<Base[]>([]);
  const [preview, setPreview] = useState<Preview | null>(null);
  const [previewErr, setPreviewErr] = useState<unknown>(null);
  // Mientras se teclea una variable a medias, el render falla; en vez de mostrar
  // el error crudo conservamos el último preview válido y avisamos suave.
  const [previewPaused, setPreviewPaused] = useState(false);
  // Datos de prueba editables para la vista previa (no se persisten): por defecto
  // un ejemplo heurístico por variable, que el usuario puede cambiar.
  const [testData, setTestData] = useState<Record<string, string>>({});
  const testValue = (name: string) => (testData[name] !== undefined ? testData[name] : sampleValue(name));
  const setTestValue = (name: string, val: string) => setTestData((d) => ({ ...d, [name]: val }));

  // Validación de campos obligatorios (inline, bajo cada campo).
  const errKey = !editing && !form.key.trim() ? "La key es obligatoria." : "";
  const errName = !form.name.trim() ? "El nombre es obligatorio." : "";
  const showErr = (f: string) => (submitAttempted || touched[f]) as boolean;
  const markTouched = (f: string) => setTouched((t) => ({ ...t, [f]: true }));

  // Carga inicial: plantilla a editar + librería base (solo EMAIL, para clonar).
  useEffect(() => {
    // ignore descarta respuestas que llegan tras cambiar de plantilla (o al
    // desmontar): sin esto, la respuesta lenta de A pisaría los datos ya
    // cargados de B (mismo patrón que el efecto de preview de abajo).
    let ignore = false;
    get<{ data: Base[] }>("/admin/base-templates")
      .then((r) => {
        if (!ignore) setBases(r.data ?? []);
      })
      .catch(() => {});
    // Origen a cargar: la plantilla a editar, o (en variante de idioma) la que se
    // clona. En ambos casos precargamos el contenido; sólo cambia qué es mutable.
    const sourceId = templateId ?? fromId;
    if (!sourceId) return;
    setLoading(true);
    get<TemplateDTO>(`${base}/templates/${sourceId}`)
      .then((t) => {
        if (ignore) return;
        setChannel(t.channel);
        setEditorMode(t.kind === "html" ? "html" : "visual");
        setForm({
          key: t.key, // variante: hereda la key del origen (queda bloqueada)
          name: t.name,
          subject: t.subject ?? "",
          // Al clonar para otro idioma arrancamos en un idioma distinto al origen.
          locale: editing ? t.locale : pickTargetLocale(t.locale),
          baseTemplateId: "",
          structure: toStructure(t.structure, t.channel),
          variables: specsFromSchema(t.requiredVariables),
          body: t.kind === "html" ? formatHtml(t.body ?? "") : (t.body ?? ""),
        });
      })
      .catch((err) => {
        if (!ignore) setLoadErr(err);
      })
      .finally(() => {
        if (!ignore) setLoading(false);
      });
    return () => {
      ignore = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [templateId, fromId]);

  // Vista previa en vivo (debounced) contra el endpoint sin guardar. En modo html
  // las variables se detectan del HTML crudo; en visual, de la estructura.
  const htmlMode = channel === "EMAIL" && editorMode === "html";
  const detected = useMemo(
    () => (htmlMode ? detectVariablesInText(`${form.body} ${form.subject}`) : detectVariables(form.structure, form.subject)),
    [htmlMode, form.body, form.structure, form.subject],
  );
  const timer = useRef<number | null>(null);
  useEffect(() => {
    // cancelled evita que una respuesta lenta pise a otra más nueva (race).
    let cancelled = false;
    if (timer.current) window.clearTimeout(timer.current);
    timer.current = window.setTimeout(async () => {
      try {
        const payload = Object.fromEntries(detected.map((v) => [v, testValue(v)]));
        const reqBody = htmlMode
          ? { channel, kind: "html", subject: form.subject, body: form.body, variables: form.variables, payload }
          : { channel, subject: form.subject, structure: form.structure, variables: form.variables, payload };
        const res = await post<Preview>(`${base}/templates/preview`, reqBody);
        if (cancelled) return;
        setPreview(res);
        setPreviewErr(null);
        setPreviewPaused(false);
      } catch (err) {
        if (cancelled) return;
        // Variable a medias / handlebars incompleto: pausa el preview sin ruido.
        if (err instanceof ApiError && err.code === "render_error") {
          setPreviewPaused(true);
        } else {
          setPreviewErr(err);
        }
      }
    }, 350);
    return () => {
      cancelled = true;
      if (timer.current) window.clearTimeout(timer.current);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [htmlMode, channel, form.subject, form.structure, form.body, form.variables, testData]);

  const cloning = !editing && !!form.baseTemplateId;
  // El desplegable de bases muestra solo las del modo actual: bloques en visual,
  // HTML en avanzado (así no hace falta marcar el nombre con «· HTML»).
  const basesForChannel = filterBasesForMode(bases, channel, editorMode);

  async function save() {
    setSubmitAttempted(true);
    if (errKey || errName) return; // errores obligatorios se ven inline
    setSaving(true);
    setSaveErr(null);
    try {
      const variables = Object.fromEntries(Object.entries(form.variables).filter(([k]) => detected.includes(k)));
      const reqBody = buildTemplateBody({ editing, htmlMode, key: form.key, channel, name: form.name, subject: form.subject, structure: form.structure, body: form.body, baseTemplateId: form.baseTemplateId, variables });
      if (!editing) reqBody.locale = form.locale; // el idioma solo se fija al crear (inmutable al versionar)
      if (editing) {
        await patch(`${base}/templates/${templateId}`, reqBody);
      } else {
        await post(`${base}/templates`, reqBody);
      }
      navigate(`/apps/${appId}?tab=templates`);
    } catch (err) {
      setSaveErr(err);
      setSaving(false);
    }
  }

  // ¿Se puede volver al editor visual? Solo si hay bloques conservados.
  const canRevert = htmlMode && structureHasBlocks(form.structure);

  // switchMode conmuta entre el editor visual y el HTML avanzado.
  function switchMode(mode: EditorMode) {
    if (mode === editorMode) return;
    if (mode === "html") {
      // Pasar a HTML: sembrar el editor con el HTML compilado (variables intactas)
      // ya formateado, o con un esqueleto por defecto si aún no hay nada.
      setForm((f) => ({ ...f, body: formatHtml(preview?.body || f.body || DEFAULT_HTML), baseTemplateId: "" }));
      setSelectedBase("");
      setEditorMode("html");
    } else {
      // Volver a visual: pide confirmación en un modal (se descarta el HTML).
      if (!structureHasBlocks(form.structure)) return;
      setRevertConfirmOpen(true);
    }
  }

  // doRevert ejecuta la vuelta a visual una vez confirmada en el modal.
  function doRevert() {
    setSelectedBase("");
    setForm((f) => ({ ...f, baseTemplateId: "" }));
    setEditorMode("visual");
    setRevertConfirmOpen(false);
  }

  // onBaseChange aplica la base elegida. Como el desplegable ya está filtrado por
  // modo, en visual son de bloques (clon en servidor) y en html precargan el editor.
  function onBaseChange(id: string) {
    setSelectedBase(id);
    const choice = resolveBaseChoice(id, basesForChannel.find((x) => x.id === id));
    setForm((f) => ({ ...f, baseTemplateId: choice.baseTemplateId, body: choice.body ? formatHtml(choice.body) : "", subject: choice.subject ?? f.subject }));
  }

  const subjectLabel = channel === "EMAIL" ? "Asunto" : channel === "SMS" ? "" : "Título";

  // Campos básicos, reutilizados en el layout de email y de no-email.
  const keyField = (
    <Field label="Key" htmlFor="tpl-key" required={!editing} error={showErr("key") ? errKey : ""} hint="Identificador estable para el backend (no se cambia después).">
      <Input id="tpl-key" placeholder={`welcome_${KEY_SUFFIX[channel] ?? "email"}`} value={form.key} disabled={editing || cloneLang} onBlur={() => markTouched("key")} onChange={(e) => setForm({ ...form, key: e.target.value })} />
    </Field>
  );
  const nameField = (
    <Field label="Nombre" htmlFor="tpl-name" required error={showErr("name") ? errName : ""} hint="Nombre visible en el panel y en los Tipos de notificación.">
      <Input id="tpl-name" placeholder="Bienvenida" value={form.name} onBlur={() => markTouched("name")} onChange={(e) => setForm({ ...form, name: e.target.value })} />
    </Field>
  );
  const localeField = (
    <Field
      label="Idioma"
      htmlFor="tpl-locale"
      required
      hint={editing ? "El idioma no se cambia al versionar." : "Crea una variante por idioma reutilizando la misma key; el envío elige por idioma y cae al de respaldo si falta."}
    >
      <Select id="tpl-locale" value={form.locale} disabled={editing} onChange={(e) => setForm({ ...form, locale: e.target.value })}>
        {LOCALES.some((l) => l.code === form.locale) ? null : <option value={form.locale}>{form.locale}</option>}
        {LOCALES.map((l) => (
          <option key={l.code} value={l.code}>
            {l.label}
          </option>
        ))}
      </Select>
    </Field>
  );
  const subjectField = subjectLabel ? (
    <Field label={subjectLabel} htmlFor="tpl-subject" hint="Admite variables. Usa «{{ }}» para insertarlas.">
      <VariableField id="tpl-subject" placeholder={channel === "EMAIL" ? "Bienvenido, {{firstName}}" : "Hola {{firstName}}"} value={form.subject} used={detected} onChange={(v) => setForm({ ...form, subject: v })} />
    </Field>
  ) : null;
  const baseField =
    !editing && !cloneLang && channel === "EMAIL" && basesForChannel.length > 0 ? (
      <Field label="Partir de una base" htmlFor="tpl-base" hint="Clona una plantilla predefinida como punto de partida.">
        <Select id="tpl-base" value={selectedBase} onChange={(e) => onBaseChange(e.target.value)}>
          <option value="">Empezar en blanco</option>
          {basesForChannel.map((b) => (
            <option key={b.id} value={b.id}>
              {b.name}
            </option>
          ))}
        </Select>
      </Field>
    ) : null;
  const cloneNote = (
    <p className="rounded-md border border-border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
      Se clonará la estructura de la plantilla base; podrás editarla después de crearla.
    </p>
  );

  return (
    <div className="space-y-5">
      <Breadcrumb
        items={[
          { label: "Aplicaciones", to: "/apps" },
          { label: "Plantillas", to: `/apps/${appId}?tab=templates` },
          { label: editing ? form.name || "Editar" : "Nueva" },
        ]}
      />

      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <Button variant="outline" size="sm" onClick={() => navigate(`/apps/${appId}?tab=templates`)}>
            <ArrowLeft />
            <span className="max-sm:sr-only">Volver</span>
          </Button>
          <div>
            <h1 className="text-xl font-semibold tracking-tight text-foreground">
              Canal <span className="text-primary">{CHANNEL_LABEL[channel] ?? channel}</span>
            </h1>
            {editing ? (
              <p className="text-sm text-muted-foreground">Guardar crea una nueva versión.</p>
            ) : cloneLang ? (
              <p className="text-sm text-muted-foreground">
                Variante de idioma de «{form.key}»: elige el idioma y traduce los textos.
              </p>
            ) : null}
          </div>
        </div>
        <Button onClick={save} disabled={saving || loading}>
          {saving ? <Loader2 className="animate-spin" /> : <Save />}
          {saving ? "Guardando…" : editing ? "Guardar versión" : "Guardar plantilla"}
        </Button>
      </div>

      <ErrorText error={loadErr} />

      {loading ? (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="size-4 animate-spin" /> Cargando plantilla…
        </div>
      ) : channel === "EMAIL" ? (
        <div className="space-y-4">
          <Card title="Datos de la plantilla">
            <div className="space-y-4">
              {/* Identidad de la plantilla: tres campos cortos en una fila. */}
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
                {keyField}
                {nameField}
                {localeField}
              </div>
              {/* Asunto a ancho completo: es el campo largo y admite variables. */}
              {subjectField}
              {baseField && <div className="sm:max-w-sm">{baseField}</div>}
              {cloning && cloneNote}
              <ErrorText error={saveErr} />
            </div>
          </Card>

          {!cloning && (
            <>
              <ModeSwitch mode={editorMode} onChange={switchMode} canRevert={canRevert} />
              {editorMode === "visual" ? (
                <EmailBuilder
                  value={form.structure}
                  onChange={(s) => setForm({ ...form, structure: s })}
                  subject={form.subject}
                  variables={form.variables}
                  onVariablesChange={(v) => setForm({ ...form, variables: v })}
                  preview={preview}
                  previewPaused={previewPaused}
                  testValue={testValue}
                  onTestChange={setTestValue}
                  footer={<SendObjectCard channel="EMAIL" payload={Object.fromEntries(detected.map((v) => [v, testValue(v)]))} />}
                />
              ) : (
                <HtmlBuilder
                  body={form.body}
                  onBodyChange={(v) => setForm({ ...form, body: v })}
                  detected={detected}
                  variables={form.variables}
                  onVariablesChange={(v) => setForm({ ...form, variables: v })}
                  preview={preview}
                  previewErr={previewErr}
                  previewPaused={previewPaused}
                  testValue={testValue}
                  onTestChange={setTestValue}
                  canRevert={canRevert}
                  onRevert={() => switchMode("visual")}
                  onFormat={() => setForm((f) => ({ ...f, body: formatHtml(f.body) }))}
                  footer={<SendObjectCard channel="EMAIL" payload={Object.fromEntries(detected.map((v) => [v, testValue(v)]))} />}
                />
              )}
            </>
          )}
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 lg:items-start">
          {/* Izquierda: editor */}
          <Card title="Editor">
            <div className="space-y-4">
              {/* Tarjeta estrecha: Key + Idioma (cortos) en una fila, Nombre debajo. */}
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                {keyField}
                {localeField}
              </div>
              {nameField}
              {subjectField}
              {cloning ? cloneNote : <ChannelContentEditor channel={channel} form={form} setForm={setForm} used={detected} />}
              <ErrorText error={saveErr} />
            </div>
          </Card>

          {/* Derecha: preview en vivo (+ objeto que se enviará) */}
          <div className="space-y-4 lg:sticky lg:top-4">
            <Card title="Vista previa">
              <ErrorText error={previewErr} />
              <PreviewPausedNotice paused={previewPaused} />
              <MissingVarsNotice missing={preview?.missingVariables} />
              <ChannelPreview channel={channel} preview={preview} />
            </Card>

            <VariablesPanel detected={detected} specs={form.variables} onChange={(v) => setForm({ ...form, variables: v })} testValue={testValue} onTestChange={setTestValue} />

            <SendObjectCard channel={channel} payload={Object.fromEntries(detected.map((v) => [v, testValue(v)]))} />
          </div>
        </div>
      )}

      {/* Confirmación (modal de la app) para volver al editor visual. */}
      <AlertDialog open={revertConfirmOpen} onOpenChange={setRevertConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Volver al editor visual</AlertDialogTitle>
            <AlertDialogDescription>
              Se descartarán los cambios de HTML y volverás al editor por bloques. Esta acción no se puede deshacer.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancelar</AlertDialogCancel>
            <AlertDialogAction onClick={doRevert}>Volver a visual</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

// ChannelContentEditor: editor de contenido específico por canal.
// recipientExample devuelve el destinatario de ejemplo según el canal.
function recipientExample(channel: string): Record<string, string> {
  if (channel === "EMAIL") return { email: "correo@ejemplo.com" };
  if (channel === "SMS") return { phone: "+15557778888" };
  return { userId: "user_123" }; // PUSH (sin proveedor) e IN_APP usan el userId
}

// SendObjectCard muestra el objeto que tu app enviaría para usar esta plantilla.
// Nota: `type` es el "Tipo de notificación" que referencia esta plantilla, NO la
// Key de la plantilla.
function SendObjectCard({ channel, payload }: { channel: string; payload: Record<string, string> }) {
  const [copied, setCopied] = useState(false);
  const obj = {
    type: "<tu Tipo de notificación>",
    recipient: recipientExample(channel),
    payload,
  };
  const json = JSON.stringify(obj, null, 2);
  const copy = () => {
    navigator.clipboard
      ?.writeText(json)
      .then(() => {
        setCopied(true);
        window.setTimeout(() => setCopied(false), 1500);
      })
      .catch(() => {});
  };
  return (
    <Card
      title="Objeto que se enviará"
      actions={
        <Button variant="outline" size="sm" onClick={copy}>
          {copied ? <Check /> : <Copy />}
          <span className="sr-only sm:not-sr-only">{copied ? "Copiado" : "Copiar"}</span>
        </Button>
      }
    >
      <p className="-mt-1 mb-2 text-xs text-muted-foreground">
        Ejemplo del cuerpo que tu app envía a <code className="rounded bg-muted px-1">POST /v1/notifications</code>.{" "}
        <span className="text-foreground">
          <code className="rounded bg-muted px-1">type</code> es el <strong>Tipo de notificación</strong> (pestaña «Tipos de notificación») que usa esta plantilla, no la Key de la plantilla.
        </span>
      </p>
      <pre className="max-h-72 overflow-auto rounded-md border border-border bg-muted/40 p-3 font-mono text-xs text-foreground">{json}</pre>
    </Card>
  );
}

function ChannelContentEditor({
  channel,
  form,
  setForm,
  used,
}: {
  channel: string;
  form: { structure: Structure; subject: string; variables: Record<string, VariableSpec> };
  setForm: (f: any) => void;
  used: string[];
}) {
  const setStructure = (structure: Structure) => setForm((f: any) => ({ ...f, structure }));

  if (channel === "SMS") {
    const body = textSection(form.structure)?.props.text ?? "";
    return (
      <Field label="Mensaje" htmlFor="sms-body" hint="Texto plano. Usa «{{ }}» para insertar variables. Ten en cuenta el límite de caracteres del SMS.">
        <VariableField
          id="sms-body"
          multiline
          className="h-32"
          value={body}
          used={used}
          onChange={(text) => setStructure({ theme: {}, sections: [{ id: "body", type: "text", props: { text } }] })}
        />
      </Field>
    );
  }

  // PUSH / IN_APP: cuerpo + acción opcional.
  const body = textSection(form.structure)?.props.text ?? "";
  const action = actionSection(form.structure)?.props ?? {};
  const rebuild = (text: string, actText: string, actUrl: string) => {
    const sections: Structure["sections"] = [{ id: "body", type: "text", props: { text } }];
    if (actText || actUrl) sections.push({ id: "action", type: "button", props: { text: actText, url: actUrl } });
    setStructure({ theme: {}, sections });
  };
  return (
    <div className="space-y-4">
      <Field label="Cuerpo" htmlFor="np-body" hint="Texto del mensaje. Usa «{{ }}» para insertar variables.">
        <VariableField id="np-body" multiline className="h-28" value={body} used={used} onChange={(text) => rebuild(text, action.text ?? "", action.url ?? "")} />
      </Field>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <Field label="Acción · texto" htmlFor="np-act-text" hint="Opcional. Etiqueta del botón / acción.">
          <VariableField id="np-act-text" placeholder="Abrir" value={action.text ?? ""} used={used} onChange={(t) => rebuild(body, t, action.url ?? "")} />
        </Field>
        <Field label="Acción · destino" htmlFor="np-act-url" hint="Opcional. URL o deep-link.">
          <VariableField id="np-act-url" placeholder="{{actionUrl}}" value={action.url ?? ""} used={used} onChange={(u) => rebuild(body, action.text ?? "", u)} />
        </Field>
      </div>
    </div>
  );
}

// ChannelPreview: renderiza la vista previa según el canal.
function ChannelPreview({ channel, preview }: { channel: string; preview: Preview | null }) {
  if (!preview) {
    return <p className="text-sm text-muted-foreground">Renderizando…</p>;
  }

  if (channel === "EMAIL") {
    return <iframe title="preview" sandbox="" className="h-[32rem] w-full rounded-lg border border-border bg-white" srcDoc={preview.html} />;
  }

  if (channel === "SMS") {
    return (
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
    );
  }

  // PUSH / IN_APP: tarjeta de notificación.
  const Icon = channel === "PUSH" ? Bell : Inbox;
  return (
    <div className="mx-auto max-w-sm rounded-xl border border-border bg-card p-3 shadow-sm">
      <div className="flex items-start gap-3">
        <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
          {channel === "PUSH" ? <Smartphone className="size-4" /> : <Icon className="size-4" />}
        </span>
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-semibold text-foreground">{preview.subject || "Título"}</p>
          <p className="mt-0.5 whitespace-pre-wrap text-sm text-muted-foreground">{preview.html || "—"}</p>
        </div>
      </div>
    </div>
  );
}
