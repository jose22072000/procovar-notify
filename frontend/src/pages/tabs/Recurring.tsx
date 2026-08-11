import { useState, type ReactNode } from "react";
import { CalendarClock, Plus, X } from "lucide-react";
import { get, post, del } from "../../api/client";
import { Badge, Card, ConfirmButton, ErrorText, Input, Label, Select, Textarea, useAsync } from "../../ui";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

interface Schedule {
  id: string;
  name: string;
  templateKey: string;
  notificationType: string;
  channel: string;
  cron: string;
  enabled: boolean;
  createdAt: string;
}

const empty = {
  name: "",
  templateKey: "",
  notificationType: "",
  channel: "EMAIL",
  email: "",
  recipientUserId: "",
  payload: "{}",
  locale: "",
  priority: "NORMAL",
  cron: "0 9 * * *",
  freq: "0 9 * * *",
};

// Presets de frecuencia → expresión cron.
const CRON_PRESETS: { label: string; value: string }[] = [
  { label: "Cada día a las 09:00", value: "0 9 * * *" },
  { label: "Cada hora", value: "0 * * * *" },
  { label: "Cada lunes a las 09:00", value: "0 9 * * 1" },
  { label: "El día 1 de cada mes a las 09:00", value: "0 9 1 * *" },
];

function Field({ label, htmlFor, required, optional, hint, children }: { label: string; htmlFor: string; required?: boolean; optional?: boolean; hint?: string; children: ReactNode }) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={htmlFor}>
        {label}
        {required && <span className="text-destructive"> *</span>}
        {optional && <span className="font-normal text-muted-foreground"> (opcional)</span>}
      </Label>
      {children}
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  );
}

export default function Recurring({ appId }: { appId: string }) {
  const base = `/admin/applications/${appId}/recurring`;
  const { data, error, loading, reload } = useAsync(() => get<{ data: Schedule[] }>(base), [appId]);
  const templates = useAsync(
    () => get<{ data: { key: string; name: string }[] }>(`/admin/applications/${appId}/templates`),
    [appId],
  );
  const routes = useAsync(
    () => get<{ data: { notificationType: string }[] }>(`/admin/applications/${appId}/routes`),
    [appId],
  );
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ ...empty });
  const [saving, setSaving] = useState(false);
  const [formErr, setFormErr] = useState<unknown>(null);
  const [actErr, setActErr] = useState<unknown>(null);

  function openNew() {
    setForm({ ...empty });
    setFormErr(null);
    setOpen(true);
  }

  async function create(e: React.FormEvent) {
    e.preventDefault();
    if (!form.templateKey) return setFormErr("Elige una plantilla.");
    if (!form.notificationType) return setFormErr("Elige un tipo de notificación.");
    // Valida el JSON antes de arrancar el envío para mostrar un mensaje claro en
    // vez del SyntaxError crudo ("Unexpected token…") del parser.
    let payload: unknown;
    try {
      payload = JSON.parse(form.payload || "{}");
    } catch {
      return setFormErr("Los datos (JSON) no son válidos. Revisa la sintaxis.");
    }
    setSaving(true);
    setFormErr(null);
    try {
      const body: Record<string, unknown> = {
        name: form.name,
        templateKey: form.templateKey,
        notificationType: form.notificationType,
        channel: form.channel,
        payload,
        priority: form.priority,
        cron: form.cron,
      };
      if (form.email) body.recipient = { email: form.email };
      if (form.recipientUserId) body.recipientUserId = form.recipientUserId;
      if (form.locale) body.locale = form.locale;
      await post(base, body);
      setOpen(false);
      reload();
    } catch (err) {
      setFormErr(err);
    } finally {
      setSaving(false);
    }
  }

  const schedules = data?.data ?? [];
  const has = schedules.length > 0;
  // Plantillas únicas por key (varias versiones/idiomas comparten key).
  const tplKeys = Array.from(new Map((templates.data?.data ?? []).map((t) => [t.key, t.name])).entries());
  // Tipos de notificación que tienen una ruta configurada.
  const routeTypes = Array.from(new Set((routes.data?.data ?? []).map((r) => r.notificationType)));

  const delAction = (s: (typeof schedules)[number]) => (
    <ConfirmButton message={`¿Eliminar la programación «${s.name}»?`} onConfirm={() => del(`${base}/${s.id}`).then(reload).catch(setActErr)}>
      Eliminar
    </ConfirmButton>
  );

  return (
    <Card
      title="Programaciones recurrentes"
      actions={
        has ? (
          <Button onClick={openNew} className="max-md:w-9 max-md:px-0">
            <Plus />
            <span className="sr-only md:not-sr-only">Nueva</span>
          </Button>
        ) : undefined
      }
    >
      <p className="-mt-1 mb-3 text-sm text-muted-foreground">Envíos automáticos que se repiten según un horario (cron): digests, recordatorios…</p>

      <ErrorText error={actErr} />
      {loading && <p className="text-sm text-muted-foreground">Cargando…</p>}
      <ErrorText error={error} />
      <ErrorText error={templates.error} />
      <ErrorText error={routes.error} />

      {data && !has && (
        <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed border-border py-10 text-center">
          <span className="flex h-12 w-12 items-center justify-center rounded-full bg-muted text-muted-foreground">
            <CalendarClock className="size-5" />
          </span>
          <div className="space-y-1">
            <p className="text-sm font-medium text-foreground">Aún no hay programaciones</p>
            <p className="text-sm text-muted-foreground">Crea un envío que se repita automáticamente.</p>
          </div>
          <Button onClick={openNew}>
            <Plus />
            Nueva
          </Button>
        </div>
      )}

      {/* Tarjetas: < 768px */}
      {has && (
        <div className="space-y-2 md:hidden">
          {schedules.map((s) => (
            <div key={s.id} className="border border-border bg-card p-3">
              <div className="min-w-0">
                <div className="flex min-w-0 items-center gap-2">
                  <span className="truncate text-sm font-medium text-foreground">{s.name}</span>
                  <Badge value={s.enabled ? "ACTIVE" : "DISABLED"} />
                </div>
                <p className="mt-1 text-xs text-muted-foreground">
                  {s.templateKey} · {s.channel} · <span className="font-mono">{s.cron}</span>
                </p>
              </div>
              <div className="mt-2 flex items-center justify-end gap-1">{delAction(s)}</div>
            </div>
          ))}
        </div>
      )}

      {/* Tabla: ≥ 768px */}
      {has && (
        <div className="hidden overflow-x-auto md:block">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-border text-xs uppercase tracking-wide text-muted-foreground">
                <th className="py-2 pr-4 font-medium">Nombre</th>
                <th className="py-2 pr-4 font-medium">Estado</th>
                <th className="py-2 pr-4 font-medium">Template</th>
                <th className="py-2 pr-4 font-medium">Canal</th>
                <th className="py-2 pr-4 font-medium">Cron</th>
                <th className="py-2 font-medium"></th>
              </tr>
            </thead>
            <tbody>
              {schedules.map((s) => (
                <tr key={s.id} className="border-b border-border/60 align-top">
                  <td className="py-2 pr-4 font-medium text-foreground">{s.name}</td>
                  <td className="py-2 pr-4">
                    <Badge value={s.enabled ? "ACTIVE" : "DISABLED"} />
                  </td>
                  <td className="py-2 pr-4 text-muted-foreground">{s.templateKey}</td>
                  <td className="py-2 pr-4 text-muted-foreground">{s.channel}</td>
                  <td className="py-2 pr-4 font-mono text-xs text-muted-foreground">{s.cron}</td>
                  <td className="py-2">
                    <div className="flex items-center justify-end gap-1">{delAction(s)}</div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Modal de alta */}
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>Nueva programación recurrente</DialogTitle>
            <DialogDescription>Un envío que se repite según el horario que elijas.</DialogDescription>
          </DialogHeader>

          <form onSubmit={create} className="space-y-4">
            <div className="-mr-2 max-h-[58vh] space-y-4 overflow-y-auto pr-2">
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <Field label="Nombre" htmlFor="rc-name" required>
                  <Input id="rc-name" placeholder="Recordatorio semanal" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
                </Field>
                <Field label="Plantilla" htmlFor="rc-tpl" required hint="La plantilla que se enviará.">
                  <Select id="rc-tpl" value={form.templateKey} onChange={(e) => setForm({ ...form, templateKey: e.target.value })}>
                    <option value="">— Elegir plantilla —</option>
                    {tplKeys.map(([key, name]) => (
                      <option key={key} value={key}>
                        {name ? `${name} (${key})` : key}
                      </option>
                    ))}
                  </Select>
                </Field>
                <Field label="Tipo de notificación" htmlFor="rc-type" required hint="Debe estar configurado en la pestaña Tipos de notificación.">
                  <Select id="rc-type" value={form.notificationType} onChange={(e) => setForm({ ...form, notificationType: e.target.value })}>
                    <option value="">— Elegir tipo —</option>
                    {routeTypes.map((t) => (
                      <option key={t} value={t}>
                        {t}
                      </option>
                    ))}
                  </Select>
                </Field>
                <Field label="Canal" htmlFor="rc-channel" required>
                  <Select id="rc-channel" value={form.channel} onChange={(e) => setForm({ ...form, channel: e.target.value })}>
                    <option value="EMAIL">EMAIL</option>
                    <option value="PUSH">PUSH</option>
                    <option value="SMS">SMS</option>
                    <option value="IN_APP">IN_APP</option>
                  </Select>
                </Field>
                <Field label="Email destinatario" htmlFor="rc-email" optional hint="Para canal EMAIL.">
                  <Input id="rc-email" type="email" placeholder="destinatario@correo.com" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} />
                </Field>
                <Field label="User ID destinatario" htmlFor="rc-user" optional hint="userId del tenant (alternativa al email).">
                  <Input id="rc-user" placeholder="u_123" value={form.recipientUserId} onChange={(e) => setForm({ ...form, recipientUserId: e.target.value })} />
                </Field>
                <Field label="Idioma" htmlFor="rc-locale" optional>
                  <Select id="rc-locale" value={form.locale} onChange={(e) => setForm({ ...form, locale: e.target.value })}>
                    <option value="">Por defecto</option>
                    <option value="es">Español (es)</option>
                    <option value="en">Inglés (en)</option>
                  </Select>
                </Field>
                <Field label="Prioridad" htmlFor="rc-priority">
                  <Select id="rc-priority" value={form.priority} onChange={(e) => setForm({ ...form, priority: e.target.value })}>
                    <option value="LOW">Baja</option>
                    <option value="NORMAL">Normal</option>
                    <option value="HIGH">Alta</option>
                    <option value="URGENT">Urgente</option>
                  </Select>
                </Field>
                <Field label="Frecuencia" htmlFor="rc-freq" hint="Cada cuánto se repite el envío.">
                  <Select
                    id="rc-freq"
                    value={form.freq}
                    onChange={(e) => {
                      const v = e.target.value;
                      setForm({ ...form, freq: v, cron: v === "custom" ? form.cron : v });
                    }}
                  >
                    {CRON_PRESETS.map((p) => (
                      <option key={p.value} value={p.value}>
                        {p.label}
                      </option>
                    ))}
                    <option value="custom">Personalizada…</option>
                  </Select>
                </Field>
                {form.freq === "custom" && (
                  <Field label="Expresión cron" htmlFor="rc-cron" required hint="Formato: minuto hora día mes díaSemana · Ej: 0 9 * * * = cada día a las 9:00.">
                    <Input id="rc-cron" className="font-mono" placeholder="0 9 * * *" value={form.cron} onChange={(e) => setForm({ ...form, cron: e.target.value })} required />
                  </Field>
                )}
              </div>

              <Field label="Datos (JSON)" htmlFor="rc-payload" hint="Variables de la plantilla.">
                <Textarea id="rc-payload" className="h-24" value={form.payload} onChange={(e) => setForm({ ...form, payload: e.target.value })} />
              </Field>

              <ErrorText error={formErr} />
            </div>

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                <X />
                Cancelar
              </Button>
              <Button type="submit" disabled={saving}>
                <Plus />
                {saving ? "Creando…" : "Crear programación"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </Card>
  );
}
