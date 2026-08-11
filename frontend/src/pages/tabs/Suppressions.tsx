import { useState, type ReactNode } from "react";
import { Ban, Plus, X } from "lucide-react";
import { get, post, del } from "../../api/client";
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

interface Suppression {
  id: string;
  channel: string;
  recipient: string;
  reason: string;
  createdAt: string;
}

const empty = { channel: "EMAIL", recipient: "", reason: "manual" };

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

export default function Suppressions({ appId }: { appId: string }) {
  const base = `/admin/applications/${appId}/suppressions`;
  const { data, error, loading, reload } = useAsync(() => get<{ data: Suppression[] }>(base), [appId]);
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

  async function add(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    setFormErr(null);
    try {
      await post(base, form);
      setOpen(false);
      reload();
    } catch (err) {
      setFormErr(err);
    } finally {
      setSaving(false);
    }
  }

  const list = data?.data ?? [];
  const has = list.length > 0;

  const delAction = (s: (typeof list)[number]) => (
    <ConfirmButton message={`¿Rehabilitar a «${s.recipient}»?`} onConfirm={() => del(`${base}/${s.id}`).then(reload).catch(setActErr)}>
      Quitar
    </ConfirmButton>
  );

  return (
    <Card
      title="Lista de supresión"
      actions={
        has ? (
          <Button onClick={openNew} className="max-md:w-9 max-md:px-0">
            <Plus />
            <span className="sr-only md:not-sr-only">Suprimir destinatario</span>
          </Button>
        ) : undefined
      }
    >
      <p className="-mt-1 mb-3 text-sm text-muted-foreground">
        Destinatarios a los que <span className="font-medium text-foreground">no</span> se envía (rebotes, quejas o manual). El
        envío los salta y la notificación queda <code>CANCELLED</code>.
      </p>

      <ErrorText error={actErr} />
      {loading && <p className="text-sm text-muted-foreground">Cargando…</p>}
      <ErrorText error={error} />

      {data && !has && (
        <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed border-border py-10 text-center">
          <span className="flex h-12 w-12 items-center justify-center rounded-full bg-muted text-muted-foreground">
            <Ban className="size-5" />
          </span>
          <div className="space-y-1">
            <p className="text-sm font-medium text-foreground">Sin destinatarios suprimidos</p>
            <p className="text-sm text-muted-foreground">Los rebotes y quejas se añaden solos; también puedes suprimir manualmente.</p>
          </div>
          <Button onClick={openNew}>
            <Plus />
            Suprimir destinatario
          </Button>
        </div>
      )}

      {/* Tarjetas: < 768px */}
      {has && (
        <div className="space-y-2 md:hidden">
          {list.map((s) => (
            <div key={s.id} className="border border-border bg-card p-3">
              <div className="min-w-0">
                <div className="flex min-w-0 items-center gap-2">
                  <span className="truncate text-sm font-medium text-foreground">{s.recipient}</span>
                  <Badge value={s.reason} />
                </div>
                <p className="mt-1 text-xs text-muted-foreground">
                  {s.channel} · suprimido el {s.createdAt?.slice(0, 10) ?? "—"}
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
                <th className="py-2 pr-4 font-medium">Destinatario</th>
                <th className="py-2 pr-4 font-medium">Motivo</th>
                <th className="py-2 pr-4 font-medium">Canal</th>
                <th className="py-2 pr-4 font-medium">Suprimido</th>
                <th className="py-2 font-medium"></th>
              </tr>
            </thead>
            <tbody>
              {list.map((s) => (
                <tr key={s.id} className="border-b border-border/60 align-top">
                  <td className="py-2 pr-4 font-medium text-foreground">{s.recipient}</td>
                  <td className="py-2 pr-4">
                    <Badge value={s.reason} />
                  </td>
                  <td className="py-2 pr-4 text-muted-foreground">{s.channel}</td>
                  <td className="py-2 pr-4 text-muted-foreground">{s.createdAt?.slice(0, 10) ?? "—"}</td>
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
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Suprimir destinatario</DialogTitle>
            <DialogDescription>No se le enviará nada por el canal elegido.</DialogDescription>
          </DialogHeader>

          <form onSubmit={add} className="space-y-4">
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field label="Canal" htmlFor="sp-channel" required>
                <Select id="sp-channel" value={form.channel} onChange={(e) => setForm({ ...form, channel: e.target.value })}>
                  <option value="EMAIL">EMAIL</option>
                  <option value="SMS">SMS</option>
                </Select>
              </Field>
              <Field label="Motivo" htmlFor="sp-reason">
                <Select id="sp-reason" value={form.reason} onChange={(e) => setForm({ ...form, reason: e.target.value })}>
                  <option value="manual">Manual</option>
                  <option value="bounce">Rebote</option>
                  <option value="complaint">Queja</option>
                </Select>
              </Field>
              <div className="sm:col-span-2">
                <Field label="Destinatario (email o teléfono)" htmlFor="sp-recipient" required>
                  <Input id="sp-recipient" placeholder={form.channel === "SMS" ? "+34600000000" : "correo@ejemplo.com"} value={form.recipient} onChange={(e) => setForm({ ...form, recipient: e.target.value })} required />
                </Field>
              </div>
            </div>

            <ErrorText error={formErr} />

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                <X />
                Cancelar
              </Button>
              <Button type="submit" disabled={saving}>
                <Ban />
                {saving ? "Suprimiendo…" : "Suprimir"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </Card>
  );
}
