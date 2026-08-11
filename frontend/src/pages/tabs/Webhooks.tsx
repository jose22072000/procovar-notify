import { useState } from "react";
import { Check, Copy, Plus, Webhook as WebhookIcon, X } from "lucide-react";
import { get, post, del } from "../../api/client";
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

interface Webhook {
  id: string;
  url: string;
  events: string[];
  status: string;
  createdAt: string;
}

const KNOWN_EVENTS: { key: string; label: string }[] = [
  { key: "notification.sent", label: "Notificación enviada" },
  { key: "notification.failed", label: "Notificación fallida" },
];

export default function Webhooks({ appId }: { appId: string }) {
  const base = `/admin/applications/${appId}/webhooks`;
  const { data, error, loading, reload } = useAsync(() => get<{ data: Webhook[] }>(base), [appId]);
  const [open, setOpen] = useState(false);
  const [url, setUrl] = useState("");
  const [events, setEvents] = useState<string[]>([]);
  const [saving, setSaving] = useState(false);
  const [formErr, setFormErr] = useState<unknown>(null);
  const [secret, setSecret] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [actErr, setActErr] = useState<unknown>(null);

  function openNew() {
    setUrl("");
    setEvents([]);
    setFormErr(null);
    setOpen(true);
  }

  function toggle(ev: string) {
    setEvents((cur) => (cur.includes(ev) ? cur.filter((e) => e !== ev) : [...cur, ev]));
  }

  async function create(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    setFormErr(null);
    try {
      const res = await post<{ secret: string }>(base, { url, events });
      setSecret(res.secret);
      setCopied(false);
      setOpen(false);
      reload();
    } catch (err) {
      setFormErr(err);
    } finally {
      setSaving(false);
    }
  }

  function copy(text: string) {
    navigator.clipboard
      ?.writeText(text)
      .then(() => {
        setCopied(true);
        setTimeout(() => setCopied(false), 1600);
      })
      .catch(() => {
        /* portapapeles denegado / contexto no seguro: sin feedback, sin romper */
      });
  }

  const hooks = data?.data ?? [];
  const has = hooks.length > 0;

  const delAction = (wh: (typeof hooks)[number]) => (
    <ConfirmButton message={`¿Eliminar este webhook?`} onConfirm={() => del(`${base}/${wh.id}`).then(reload).catch(setActErr)}>
      Eliminar
    </ConfirmButton>
  );

  return (
    <Card
      title="Webhooks"
      actions={
        has ? (
          <Button onClick={openNew} className="max-md:w-9 max-md:px-0">
            <Plus />
            <span className="sr-only md:not-sr-only">Nuevo</span>
          </Button>
        ) : undefined
      }
    >
      <p className="-mt-1 mb-3 text-sm text-muted-foreground">
        QBNotify avisa a tu backend (POST firmado) cuando cambia el estado de una notificación: enviada, fallida…
      </p>

      {secret && (
        <div className="mb-4 rounded-lg border border-amber-300 bg-amber-50 p-3 text-sm">
          <p className="font-medium text-amber-800">Guarda el secreto de firma ahora — no se vuelve a mostrar.</p>
          <div className="mt-2 flex items-center gap-2">
            <code className="flex-1 break-all rounded bg-card px-2 py-1 font-mono text-xs">{secret}</code>
            <Button variant="outline" size="sm" onClick={() => copy(secret)}>
              {copied ? <Check /> : <Copy />}
              {copied ? "Copiado" : "Copiar"}
            </Button>
          </div>
          <p className="mt-2 text-xs text-amber-700">
            Verifica cada POST con <code>X-QBN-Signature = HMAC-SHA256(secret, "&lt;timestamp&gt;.&lt;body&gt;")</code>.
          </p>
        </div>
      )}

      <ErrorText error={actErr} />
      {loading && <p className="text-sm text-muted-foreground">Cargando…</p>}
      <ErrorText error={error} />

      {data && !has && (
        <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed border-border py-10 text-center">
          <span className="flex h-12 w-12 items-center justify-center rounded-full bg-muted text-muted-foreground">
            <WebhookIcon className="size-5" />
          </span>
          <div className="space-y-1">
            <p className="text-sm font-medium text-foreground">Aún no hay webhooks</p>
            <p className="text-sm text-muted-foreground">Registra una URL para recibir avisos de estado en tu backend.</p>
          </div>
          <Button onClick={openNew}>
            <Plus />
            Nuevo
          </Button>
        </div>
      )}

      {/* Tarjetas: < 768px */}
      {has && (
        <div className="space-y-2 md:hidden">
          {hooks.map((wh) => (
            <div key={wh.id} className="border border-border bg-card p-3">
              <div className="min-w-0">
                <div className="flex min-w-0 items-center gap-2">
                  <span className="truncate font-mono text-xs text-foreground">{wh.url}</span>
                  <Badge value={wh.status} />
                </div>
                <p className="mt-1 text-xs text-muted-foreground">
                  Eventos: {wh.events.length ? wh.events.join(", ") : "todos"}
                </p>
              </div>
              <div className="mt-2 flex items-center justify-end gap-1">{delAction(wh)}</div>
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
                <th className="py-2 pr-4 font-medium">URL</th>
                <th className="py-2 pr-4 font-medium">Estado</th>
                <th className="py-2 pr-4 font-medium">Eventos</th>
                <th className="py-2 font-medium"></th>
              </tr>
            </thead>
            <tbody>
              {hooks.map((wh) => (
                <tr key={wh.id} className="border-b border-border/60 align-top">
                  <td className="py-2 pr-4 font-mono text-xs text-foreground">{wh.url}</td>
                  <td className="py-2 pr-4">
                    <Badge value={wh.status} />
                  </td>
                  <td className="py-2 pr-4 text-muted-foreground">{wh.events.length ? wh.events.join(", ") : "todos"}</td>
                  <td className="py-2">
                    <div className="flex items-center justify-end gap-1">{delAction(wh)}</div>
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
            <DialogTitle>Nuevo webhook</DialogTitle>
            <DialogDescription>Recibe avisos de estado en tu backend.</DialogDescription>
          </DialogHeader>

          <form onSubmit={create} className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="wh-url">
                URL <span className="text-destructive">*</span>
              </Label>
              <Input id="wh-url" type="url" placeholder="https://tu-backend.com/webhooks/qbnotify" value={url} onChange={(e) => setUrl(e.target.value)} required />
              <p className="text-xs text-muted-foreground">Endpoint de tu backend que recibirá los POST firmados.</p>
            </div>

            <div className="space-y-1.5">
              <Label>Eventos</Label>
              <div className="space-y-1.5 rounded-md border border-border p-3">
                {KNOWN_EVENTS.map((ev) => (
                  <label key={ev.key} className="flex items-center gap-2 text-sm">
                    <input type="checkbox" className="size-4 rounded border-input accent-primary" checked={events.includes(ev.key)} onChange={() => toggle(ev.key)} />
                    <span className="text-foreground">{ev.label}</span>
                    <code className="text-xs text-muted-foreground">{ev.key}</code>
                  </label>
                ))}
              </div>
              <p className="text-xs text-muted-foreground">Si no marcas ninguno, recibirás todos los eventos.</p>
            </div>

            <ErrorText error={formErr} />

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                <X />
                Cancelar
              </Button>
              <Button type="submit" disabled={saving}>
                <Plus />
                {saving ? "Creando…" : "Crear webhook"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </Card>
  );
}
