import { useState, type ReactNode } from "react";
import { Plus, Route as RouteIcon, Settings2, X } from "lucide-react";
import { get, post, patch, del } from "../../api/client";
import { Badge, Card, ConfirmButton, ErrorText, Input, Label, Select, useAsync } from "../../ui";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

interface Route {
  id: string;
  notificationType: string;
  channel: string;
  templateKey?: string;
  smtpConnectionId?: string;
  channelProviderId?: string;
  sendPriority?: string;
  piiRetentionDays?: number | null;
  retentionDays?: number | null;
}
interface Smtp {
  id: string;
  name: string;
}
interface Provider {
  id: string;
  name: string;
  channel: string;
}
interface Template {
  key: string;
  name: string;
  channel: string;
  locale: string;
}

const CHANNELS = ["EMAIL", "PUSH", "SMS", "IN_APP"] as const;

// Prioridad de envío del tipo: a qué cola van sus notificaciones. URGENT/HIGH
// van a la cola crítica (OTP, recuperar contraseña…); LOW a la de fondo.
const PRIORITIES = [
  { value: "URGENT", label: "Urgente — el destinatario está esperando (OTP, recuperar contraseña)" },
  { value: "HIGH", label: "Alta — importante, con preferencia sobre el resto" },
  { value: "NORMAL", label: "Normal — envíos habituales" },
  { value: "LOW", label: "Baja — masivos o de fondo (newsletters, resúmenes)" },
] as const;

const empty = { notificationType: "", channel: "EMAIL", templateKey: "", smtpConnectionId: "", channelProviderId: "", sendPriority: "NORMAL", piiRetentionDays: "", retentionDays: "" };

// daysOrNull convierte el texto de un input numérico opcional a int o null.
const daysOrNull = (s: string) => (s.trim() === "" ? null : Number(s));

// Resumen corto de la retención de un tipo para el listado.
const retentionLabel = (r: Route) => {
  const pii = r.piiRetentionDays ? `PII ${r.piiRetentionDays}d` : "PII app";
  const del = r.retentionDays ? `borra ${r.retentionDays}d` : "no borra";
  return `${pii} · ${del}`;
};

function Field({ label, htmlFor, required, hint, children }: { label: string; htmlFor: string; required?: boolean; hint?: string; children: ReactNode }) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={htmlFor}>
        {label}
        {required && <span className="text-destructive"> *</span>}
      </Label>
      {children}
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  );
}

export default function RoutesTab({ appId }: { appId: string }) {
  const base = `/admin/applications/${appId}`;
  const routes = useAsync(() => get<{ data: Route[] }>(`${base}/routes`), [appId]);
  const smtp = useAsync(() => get<{ data: Smtp[] }>(`${base}/smtp`), [appId]);
  const providers = useAsync(() => get<{ data: Provider[] }>(`${base}/providers`), [appId]);
  const templates = useAsync(() => get<{ data: Template[] }>(`${base}/templates`), [appId]);
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

  // Al cambiar de canal se reinician plantilla y destino (dependen del canal);
  // nombre y prioridad se conservan (no dependen del canal).
  function changeChannel(channel: string) {
    setForm({
      ...empty,
      notificationType: form.notificationType,
      sendPriority: form.sendPriority,
      piiRetentionDays: form.piiRetentionDays,
      retentionDays: form.retentionDays,
      channel,
    });
  }

  async function create(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    setFormErr(null);
    try {
      const isEmail = form.channel === "EMAIL";
      const usesProvider = form.channel === "SMS" || form.channel === "PUSH";
      await post(`${base}/routes`, {
        notificationType: form.notificationType,
        channel: form.channel,
        templateKey: form.templateKey,
        smtpConnectionId: isEmail ? form.smtpConnectionId || null : null,
        channelProviderId: usesProvider ? form.channelProviderId || null : null,
        sendPriority: form.sendPriority,
        piiRetentionDays: daysOrNull(form.piiRetentionDays),
        retentionDays: daysOrNull(form.retentionDays),
      });
      setOpen(false);
      routes.reload();
    } catch (err) {
      setFormErr(err);
    } finally {
      setSaving(false);
    }
  }

  // Listas dependientes del canal seleccionado en el formulario.
  const templatesForChannel = Array.from(
    new Map((templates.data?.data ?? []).filter((t) => t.channel === form.channel).map((t) => [t.key, t])).values(),
  );
  const providersForChannel = (providers.data?.data ?? []).filter((p) => p.channel === form.channel);

  // Idiomas disponibles por key: una key agrupa todas sus variantes de idioma,
  // así que elegir la plantilla en la ruta cubre todos sus idiomas a la vez.
  const localesByKey = new Map<string, string[]>();
  for (const t of templates.data?.data ?? []) {
    const locs = localesByKey.get(t.key) ?? [];
    if (!locs.includes(t.locale)) locs.push(t.locale);
    localesByKey.set(t.key, locs);
  }
  const tplLocales = (key?: string) => (key ? (localesByKey.get(key) ?? []) : []);

  // Helpers para mostrar nombres (no ids) en el listado.
  const tplName = (key?: string) => templates.data?.data?.find((t) => t.key === key)?.name ?? key ?? "—";
  const smtpName = (id?: string) => smtp.data?.data?.find((s) => s.id === id)?.name;
  const provName = (id?: string) => providers.data?.data?.find((p) => p.id === id)?.name;
  const destLabel = (r: Route) => {
    if (r.channel === "EMAIL") return smtpName(r.smtpConnectionId) ?? "—";
    if (r.channel === "SMS") return provName(r.channelProviderId) ?? "—";
    if (r.channel === "PUSH") return provName(r.channelProviderId) ?? "Bandeja interna";
    return "Bandeja interna";
  };

  const list = routes.data?.data ?? [];
  const hasRoutes = list.length > 0;

  // Ajustes operables del tipo (prioridad + retención): editables sin recrearlo.
  const [editRoute, setEditRoute] = useState<Route | null>(null);
  const [editForm, setEditForm] = useState({ sendPriority: "NORMAL", piiRetentionDays: "", retentionDays: "" });
  const [editErr, setEditErr] = useState<unknown>(null);
  const [savingEdit, setSavingEdit] = useState(false);

  function openSettings(r: Route) {
    setEditRoute(r);
    setEditErr(null);
    setEditForm({
      sendPriority: r.sendPriority ?? "NORMAL",
      piiRetentionDays: r.piiRetentionDays ? String(r.piiRetentionDays) : "",
      retentionDays: r.retentionDays ? String(r.retentionDays) : "",
    });
  }

  async function saveSettings(e: React.FormEvent) {
    e.preventDefault();
    if (!editRoute) return;
    setSavingEdit(true);
    setEditErr(null);
    try {
      // El PATCH lleva siempre los tres campos: null en retención es un valor
      // válido (heredar / no borrar), no "sin cambio".
      await patch(`${base}/routes/${editRoute.id}`, {
        sendPriority: editForm.sendPriority,
        piiRetentionDays: daysOrNull(editForm.piiRetentionDays),
        retentionDays: daysOrNull(editForm.retentionDays),
      });
      setEditRoute(null);
      routes.reload();
    } catch (err) {
      setEditErr(err);
    } finally {
      setSavingEdit(false);
    }
  }

  const settingsAction = (r: Route) => (
    <Button variant="outline" size="sm" onClick={() => openSettings(r)} title="Prioridad y retención">
      <Settings2 />
      <span className="sr-only lg:not-sr-only">Ajustes</span>
    </Button>
  );

  const delAction = (r: Route) => (
    <ConfirmButton
      message={`¿Eliminar el tipo de notificación «${r.notificationType}»?`}
      onConfirm={() => del(`${base}/routes/${r.id}`).then(routes.reload).catch(setActErr)}
    >
      Eliminar
    </ConfirmButton>
  );

  return (
    <Card
      title="Tipos de notificación"
      actions={
        hasRoutes ? (
          <Button onClick={openNew} className="max-md:w-9 max-md:px-0">
            <Plus />
            <span className="sr-only md:not-sr-only">Nuevo</span>
          </Button>
        ) : undefined
      }
    >
      <p className="-mt-1 mb-3 text-sm text-muted-foreground">
        Cada tipo define un envío completo: <span className="font-medium text-foreground">nombre → canal → plantilla → destino</span>. Tu app lo dispara por nombre o id.
      </p>

      <ErrorText error={actErr} />
      <ErrorText error={routes.error} />
      <ErrorText error={smtp.error} />
      <ErrorText error={providers.error} />
      <ErrorText error={templates.error} />

      {routes.data && !hasRoutes && (
        <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed border-border py-10 text-center">
          <span className="flex h-12 w-12 items-center justify-center rounded-full bg-muted text-muted-foreground">
            <RouteIcon className="size-5" />
          </span>
          <div className="space-y-1">
            <p className="text-sm font-medium text-foreground">Aún no hay tipos de notificación</p>
            <p className="text-sm text-muted-foreground">Crea uno para indicar canal, plantilla y por dónde se envía.</p>
          </div>
          <Button onClick={openNew}>
            <Plus />
            Nuevo
          </Button>
        </div>
      )}

      {/* Tarjetas: < 768px */}
      {hasRoutes && (
        <div className="space-y-2 md:hidden">
          {list.map((r) => (
            <div key={r.id} className="border border-border bg-card p-3">
              <div className="min-w-0">
                <div className="flex min-w-0 flex-wrap items-center gap-2">
                  <span className="truncate text-sm font-medium text-foreground">{r.notificationType}</span>
                  <Badge value={r.channel} />
                  {(r.sendPriority === "URGENT" || r.sendPriority === "HIGH") && <Badge value={r.sendPriority} />}
                </div>
                <p className="mt-1 text-xs text-muted-foreground">
                  Plantilla: {tplName(r.templateKey)}
                  {tplLocales(r.templateKey).length > 0 && ` (${tplLocales(r.templateKey).join(", ")})`} · Destino: {destLabel(r)} · {retentionLabel(r)}
                </p>
              </div>
              <div className="mt-2 flex items-center justify-end gap-1">
                {settingsAction(r)}
                {delAction(r)}
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Tabla: ≥ 768px */}
      {hasRoutes && (
        <div className="hidden overflow-x-auto md:block">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-border text-xs uppercase tracking-wide text-muted-foreground">
                <th className="py-2 pr-4 font-medium">Nombre</th>
                <th className="py-2 pr-4 font-medium">Canal</th>
                <th className="py-2 pr-4 font-medium">Plantilla</th>
                <th className="py-2 pr-4 font-medium">Destino</th>
                <th className="py-2 pr-4 font-medium">Prioridad</th>
                <th className="py-2 pr-4 font-medium">Retención</th>
                <th className="py-2 font-medium"></th>
              </tr>
            </thead>
            <tbody>
              {list.map((r) => (
                <tr key={r.id} className="border-b border-border/60 align-top">
                  <td className="py-2 pr-4">
                    <span className="font-medium text-foreground">{r.notificationType}</span>
                  </td>
                  <td className="py-2 pr-4">
                    <Badge value={r.channel} />
                  </td>
                  <td className="py-2 pr-4 text-muted-foreground">
                    {tplName(r.templateKey)}
                    {tplLocales(r.templateKey).length > 0 && (
                      <span className="ml-1.5 inline-flex flex-wrap gap-1 align-middle">
                        {tplLocales(r.templateKey).map((loc) => (
                          <Badge key={loc} value={loc} />
                        ))}
                      </span>
                    )}
                  </td>
                  <td className="py-2 pr-4 text-muted-foreground">{destLabel(r)}</td>
                  <td className="py-2 pr-4">
                    <Badge value={r.sendPriority ?? "NORMAL"} />
                  </td>
                  <td className="py-2 pr-4 text-xs text-muted-foreground">{retentionLabel(r)}</td>
                  <td className="py-2">
                    <div className="flex items-center justify-end gap-1">
                      {settingsAction(r)}
                      {delAction(r)}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Modal de alta */}
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Nuevo tipo de notificación</DialogTitle>
            <DialogDescription>Nombre, canal, plantilla y destino: todo lo que necesita un envío.</DialogDescription>
          </DialogHeader>

          <form onSubmit={create} className="space-y-4">
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field label="Nombre" htmlFor="rt-name" required hint="Identificador único (welcome_email, reset_sms…). Tu app lo usa para disparar el envío.">
                <Input id="rt-name" placeholder="welcome_email" value={form.notificationType} onChange={(e) => setForm({ ...form, notificationType: e.target.value })} required />
              </Field>
              <Field label="Canal" htmlFor="rt-channel" required>
                <Select id="rt-channel" value={form.channel} onChange={(e) => changeChannel(e.target.value)}>
                  {CHANNELS.map((c) => (
                    <option key={c} value={c}>
                      {c}
                    </option>
                  ))}
                </Select>
              </Field>

              <Field
                label="Plantilla"
                htmlFor="rt-template"
                required
                hint="La plantilla incluye todos sus idiomas: el envío usa el «locale» del mensaje y cae al de respaldo si falta. Solo se muestran las del canal elegido."
              >
                <Select id="rt-template" value={form.templateKey} onChange={(e) => setForm({ ...form, templateKey: e.target.value })}>
                  <option value="">— Elegir plantilla —</option>
                  {templatesForChannel.map((t) => (
                    <option key={t.key} value={t.key}>
                      {t.name} — {tplLocales(t.key).join(", ")}
                    </option>
                  ))}
                </Select>
              </Field>

              {form.channel === "EMAIL" && (
                <Field label="Conexión SMTP" htmlFor="rt-smtp" required hint="Buzón por el que saldrán estos emails.">
                  <Select id="rt-smtp" value={form.smtpConnectionId} onChange={(e) => setForm({ ...form, smtpConnectionId: e.target.value })}>
                    <option value="">— Elegir conexión —</option>
                    {smtp.data?.data?.map((s) => (
                      <option key={s.id} value={s.id}>
                        {s.name}
                      </option>
                    ))}
                  </Select>
                </Field>
              )}

              {form.channel === "SMS" && (
                <Field label="Proveedor" htmlFor="rt-provider" required hint="Proveedor SMS (p. ej. Twilio) por el que se envía.">
                  <Select id="rt-provider" value={form.channelProviderId} onChange={(e) => setForm({ ...form, channelProviderId: e.target.value })}>
                    <option value="">— Elegir proveedor —</option>
                    {providersForChannel.map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.name}
                      </option>
                    ))}
                  </Select>
                </Field>
              )}

              {form.channel === "PUSH" && (
                <Field label="Proveedor" htmlFor="rt-provider" hint="Opcional. Sin proveedor, la notificación se entrega como bandeja interna (la app la lee abierta); configura FCM/APNS más adelante para llegar al dispositivo.">
                  <Select id="rt-provider" value={form.channelProviderId} onChange={(e) => setForm({ ...form, channelProviderId: e.target.value })}>
                    <option value="">Sin proveedor (bandeja interna)</option>
                    {providersForChannel.map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.name}
                      </option>
                    ))}
                  </Select>
                </Field>
              )}
              <Field
                label="Prioridad de envío"
                htmlFor="rt-priority"
                hint="Urgente/Alta van a la cola crítica: úsalas cuando alguien espera el mensaje. Tu app puede sobreescribirla por envío."
              >
                <Select id="rt-priority" value={form.sendPriority} onChange={(e) => setForm({ ...form, sendPriority: e.target.value })}>
                  {PRIORITIES.map((p) => (
                    <option key={p.value} value={p.value}>
                      {p.label}
                    </option>
                  ))}
                </Select>
              </Field>

              <Field label="Anonimizar PII (días)" htmlFor="rt-pii" hint="Vacía datos personales pasado el plazo. Vacío = usa el valor de la aplicación.">
                <Input id="rt-pii" type="number" min={1} placeholder="90" value={form.piiRetentionDays} onChange={(e) => setForm({ ...form, piiRetentionDays: e.target.value })} />
              </Field>
              <Field label="Borrar del todo (días)" htmlFor="rt-del" hint="Elimina la notificación con sus intentos y logs (ahorra disco). Vacío = no se borra nunca.">
                <Input id="rt-del" type="number" min={1} placeholder="365" value={form.retentionDays} onChange={(e) => setForm({ ...form, retentionDays: e.target.value })} />
              </Field>
            </div>

            {templatesForChannel.length === 0 && (
              <p className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800">
                No hay plantillas para el canal {form.channel}. Crea una en la pestaña «Plantillas» antes de definir este tipo.
              </p>
            )}
            {form.channel === "IN_APP" && (
              <p className="rounded-md border border-border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
                IN_APP no lleva destino externo: la notificación se guarda y tu app la lee por API (bandeja de entrada).
              </p>
            )}

            <ErrorText error={formErr} />

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                <X />
                Cancelar
              </Button>
              <Button type="submit" disabled={saving}>
                <Plus />
                {saving ? "Creando…" : "Crear tipo"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Ajustes del tipo: prioridad y retención (lo editable sin recrearlo). */}
      <Dialog open={!!editRoute} onOpenChange={(o) => !o && setEditRoute(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Ajustes · {editRoute?.notificationType}</DialogTitle>
            <DialogDescription>Prioridad de envío y retención de las notificaciones de este tipo.</DialogDescription>
          </DialogHeader>

          <form onSubmit={saveSettings} className="space-y-4">
            <Field
              label="Prioridad de envío"
              htmlFor="ed-priority"
              hint="Urgente/Alta van a la cola crítica: úsalas cuando alguien espera el mensaje."
            >
              <Select id="ed-priority" value={editForm.sendPriority} onChange={(e) => setEditForm({ ...editForm, sendPriority: e.target.value })}>
                {PRIORITIES.map((p) => (
                  <option key={p.value} value={p.value}>
                    {p.label}
                  </option>
                ))}
              </Select>
            </Field>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field label="Anonimizar PII (días)" htmlFor="ed-pii" hint="Vacío = usa el valor de la aplicación.">
                <Input id="ed-pii" type="number" min={1} placeholder="90" value={editForm.piiRetentionDays} onChange={(e) => setEditForm({ ...editForm, piiRetentionDays: e.target.value })} />
              </Field>
              <Field label="Borrar del todo (días)" htmlFor="ed-del" hint="Vacío = no se borra nunca.">
                <Input id="ed-del" type="number" min={1} placeholder="365" value={editForm.retentionDays} onChange={(e) => setEditForm({ ...editForm, retentionDays: e.target.value })} />
              </Field>
            </div>

            <ErrorText error={editErr} />

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setEditRoute(null)}>
                <X />
                Cancelar
              </Button>
              <Button type="submit" disabled={savingEdit}>
                {savingEdit ? "Guardando…" : "Guardar"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </Card>
  );
}
