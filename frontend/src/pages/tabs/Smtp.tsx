import { useState, type ReactNode } from "react";
import { AlertCircle, CheckCircle2, Loader2, Mail, Pencil, Plus, Save, X, Zap } from "lucide-react";
import { get, post, patch, del, ApiError } from "../../api/client";
import { Badge, Card, ConfirmButton, ErrorText, Input, Label, useAsync } from "../../ui";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

interface Smtp {
  id: string;
  name: string;
  host: string;
  port: number;
  username: string;
  fromEmail: string;
  fromName: string;
  secure: boolean;
  status: string;
}

const empty = { name: "", host: "", port: "", username: "", password: "", fromEmail: "", fromName: "", secure: false };

function Field({ label, htmlFor, required, children }: { label: string; htmlFor: string; required?: boolean; children: ReactNode }) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={htmlFor}>
        {label}
        {required && <span className="text-destructive"> *</span>}
      </Label>
      {children}
    </div>
  );
}

export default function SmtpTab({ appId }: { appId: string }) {
  const base = `/admin/applications/${appId}/smtp`;
  const { data, error, loading, reload } = useAsync(() => get<{ data: Smtp[] }>(base), [appId]);
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ ...empty });
  const [editId, setEditId] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [formErr, setFormErr] = useState<unknown>(null);
  const [testRes, setTestRes] = useState<{ ok: boolean; message: string } | null>(null);
  const [testingId, setTestingId] = useState<string | null>(null);
  const [actErr, setActErr] = useState<unknown>(null);

  function openNew() {
    setEditId(null);
    setForm({ ...empty });
    setFormErr(null);
    setOpen(true);
  }

  function openEdit(s: Smtp) {
    setEditId(s.id);
    setForm({ name: s.name, host: s.host, port: String(s.port), username: s.username, password: "", fromEmail: s.fromEmail, fromName: s.fromName, secure: s.secure });
    setFormErr(null);
    setOpen(true);
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    setFormErr(null);
    try {
      const body = { ...form, port: Number(form.port) };
      if (editId) await patch(`${base}/${editId}`, body);
      else await post(base, body);
      setOpen(false);
      reload();
    } catch (err) {
      setFormErr(err);
    } finally {
      setSaving(false);
    }
  }

  async function test(s: Smtp) {
    setTestRes(null);
    setTestingId(s.id);
    try {
      await post(`${base}/${s.id}/test`);
      setTestRes({ ok: true, message: `Conexión correcta con «${s.name}».` });
    } catch (err) {
      const detail = err instanceof ApiError ? err.message : "No se pudo conectar.";
      setTestRes({ ok: false, message: `Falló la conexión con «${s.name}»: ${detail}` });
    } finally {
      setTestingId(null);
    }
  }

  const conns = data?.data ?? [];
  const hasConns = conns.length > 0;

  const rowActions = (s: Smtp) => (
    <>
      <Button variant="outline" size="sm" className="max-lg:w-8 max-lg:px-0" onClick={() => openEdit(s)}>
        <Pencil />
        <span className="sr-only lg:not-sr-only">Editar</span>
      </Button>
      <Button variant="outline" size="sm" className="max-lg:w-8 max-lg:px-0" onClick={() => test(s)} disabled={testingId === s.id}>
        {testingId === s.id ? <Loader2 className="animate-spin" /> : <Zap />}
        <span className="sr-only lg:not-sr-only">{testingId === s.id ? "Probando…" : "Probar"}</span>
      </Button>
      <ConfirmButton message={`¿Eliminar la conexión «${s.name}»?`} onConfirm={() => del(`${base}/${s.id}`).then(reload).catch(setActErr)}>
        Eliminar
      </ConfirmButton>
    </>
  );

  return (
    <Card
      title="Conexiones SMTP"
      actions={
        hasConns ? (
          <Button onClick={openNew} className="max-md:w-9 max-md:px-0">
            <Plus />
            <span className="sr-only md:not-sr-only">Nueva</span>
          </Button>
        ) : undefined
      }
    >
      <p className="-mt-1 mb-3 text-sm text-muted-foreground">Servidores de correo desde los que esta aplicación envía sus emails.</p>

      {testRes && (
        <div
          className={`mb-4 flex items-start gap-2 rounded-lg border px-3 py-2.5 text-sm ${
            testRes.ok ? "border-emerald-200 bg-emerald-50 text-emerald-800" : "border-destructive/30 bg-destructive/10 text-destructive"
          }`}
        >
          {testRes.ok ? <CheckCircle2 className="mt-0.5 size-4 shrink-0" /> : <AlertCircle className="mt-0.5 size-4 shrink-0" />}
          <span className="flex-1">{testRes.message}</span>
          <button onClick={() => setTestRes(null)} aria-label="Cerrar aviso" className="opacity-60 transition-opacity hover:opacity-100">
            <X className="size-4" />
          </button>
        </div>
      )}

      <ErrorText error={actErr} />
      {loading && <p className="text-sm text-muted-foreground">Cargando…</p>}
      <ErrorText error={error} />

      {data && !hasConns && !loading && (
        <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed border-border py-10 text-center">
          <span className="flex h-12 w-12 items-center justify-center rounded-full bg-muted text-muted-foreground">
            <Mail className="size-5" />
          </span>
          <div className="space-y-1">
            <p className="text-sm font-medium text-foreground">Aún no hay conexiones SMTP</p>
            <p className="text-sm text-muted-foreground">Configura un servidor de correo para poder enviar emails.</p>
          </div>
          <Button onClick={openNew}>
            <Plus />
            Nueva
          </Button>
        </div>
      )}

      {/* Tarjetas: < 768px */}
      {hasConns && (
        <div className="space-y-2 md:hidden">
          {conns.map((s) => (
            <div key={s.id} className="border border-border bg-card p-3">
              <div className="min-w-0">
                <div className="flex min-w-0 items-center gap-2">
                  <span className="truncate text-sm font-medium text-foreground">{s.name}</span>
                  <Badge value={s.status} />
                </div>
                <p className="mt-1 break-all font-mono text-xs text-muted-foreground">
                  {s.host}:{s.port}
                  {s.secure ? " · TLS" : ""}
                </p>
                <p className="break-all font-mono text-xs text-muted-foreground">
                  {s.fromName ? `${s.fromName} ` : ""}&lt;{s.fromEmail}&gt;
                </p>
              </div>
              <div className="mt-2 flex items-center justify-end gap-1">{rowActions(s)}</div>
            </div>
          ))}
        </div>
      )}

      {/* Tabla: ≥ 768px */}
      {hasConns && (
        <div className="hidden overflow-x-auto md:block">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-border text-xs uppercase tracking-wide text-muted-foreground">
                <th className="py-2 pr-4 font-medium">Nombre</th>
                <th className="py-2 pr-4 font-medium">Estado</th>
                <th className="py-2 pr-4 font-medium">Servidor</th>
                <th className="py-2 pr-4 font-medium">Remitente</th>
                <th className="py-2 font-medium"></th>
              </tr>
            </thead>
            <tbody>
              {conns.map((s) => (
                <tr key={s.id} className="border-b border-border/60 align-top">
                  <td className="py-2 pr-4 font-medium text-foreground">{s.name}</td>
                  <td className="py-2 pr-4">
                    <Badge value={s.status} />
                  </td>
                  <td className="py-2 pr-4 font-mono text-xs text-muted-foreground">
                    {s.host}:{s.port}
                    {s.secure ? " · TLS" : ""}
                  </td>
                  <td className="py-2 pr-4 font-mono text-xs text-muted-foreground">
                    {s.fromName ? `${s.fromName} ` : ""}&lt;{s.fromEmail}&gt;
                  </td>
                  <td className="py-2">
                    <div className="flex items-center justify-end gap-1">{rowActions(s)}</div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Modal de alta / edición */}
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>{editId ? "Editar conexión SMTP" : "Nueva conexión SMTP"}</DialogTitle>
            <DialogDescription>Datos del servidor de correo de salida.</DialogDescription>
          </DialogHeader>

          <form onSubmit={submit} className="space-y-4" autoComplete="off">
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field label="Nombre" htmlFor="smtp-name" required>
                <Input id="smtp-name" placeholder="transaccional" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
              </Field>
              <Field label="Host" htmlFor="smtp-host" required>
                <Input id="smtp-host" placeholder="smtp.tu-proveedor.com" value={form.host} onChange={(e) => setForm({ ...form, host: e.target.value })} required />
              </Field>
              <Field label="Puerto" htmlFor="smtp-port" required>
                <Input id="smtp-port" type="number" placeholder="587" value={form.port} onChange={(e) => setForm({ ...form, port: e.target.value })} required />
              </Field>
              <Field label="Usuario" htmlFor="smtp-user">
                <Input id="smtp-user" name="smtp-user" autoComplete="off" placeholder="usuario@tu-proveedor.com" value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} />
              </Field>
              <Field label="Contraseña" htmlFor="smtp-pass">
                <Input id="smtp-pass" name="smtp-pass" type="password" autoComplete="new-password" placeholder={editId ? "(sin cambios)" : "••••••••"} value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} />
              </Field>
              <Field label="Email remitente (From)" htmlFor="smtp-from" required>
                <Input id="smtp-from" type="email" placeholder="noreply@tudominio.com" value={form.fromEmail} onChange={(e) => setForm({ ...form, fromEmail: e.target.value })} required />
              </Field>
              <Field label="Nombre remitente" htmlFor="smtp-fromname">
                <Input id="smtp-fromname" placeholder="Tu Marca" value={form.fromName} onChange={(e) => setForm({ ...form, fromName: e.target.value })} />
              </Field>

              <div className="space-y-1.5 sm:col-span-2">
                <label className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    className="size-4 rounded border-input accent-primary"
                    checked={form.secure}
                    onChange={(e) => setForm({ ...form, secure: e.target.checked })}
                  />
                  <span className="text-sm font-medium text-foreground">Conexión segura (TLS/SSL)</span>
                </label>
                <p className="text-xs text-muted-foreground">Cifra la conexión: SSL en el puerto 465, STARTTLS en el 587. Para pruebas locales, desactívalo.</p>
              </div>
            </div>

            <ErrorText error={formErr} />

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                <X />
                Cancelar
              </Button>
              <Button type="submit" disabled={saving}>
                {editId ? <Save /> : <Plus />}
                {saving ? "Guardando…" : editId ? "Guardar cambios" : "Crear conexión"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </Card>
  );
}
