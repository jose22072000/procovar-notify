import { useState } from "react";
import { Check, Copy, KeyRound, Plus } from "lucide-react";
import { get, post, del } from "../../api/client";
import { Badge, Card, ConfirmButton, ErrorText, Input, Label, Table, useAsync } from "../../ui";
import { Button } from "@/components/ui/button";

interface ApiKey {
  id: string;
  name: string;
  keyId: string;
  scopes: string[];
  status: string;
  lastUsedAt?: string;
  createdAt: string;
}

const SCOPE_SEND = "notifications:send";
const SCOPE_READ = "notifications:read";

export default function ApiKeys({ appId }: { appId: string }) {
  const { data, error, loading, reload } = useAsync(
    () => get<{ data: ApiKey[] }>(`/admin/applications/${appId}/api-keys`),
    [appId],
  );
  const [secret, setSecret] = useState<{ keyId: string; secret: string } | null>(null);
  const [copied, setCopied] = useState(false);
  const [actErr, setActErr] = useState<unknown>(null);
  const [saving, setSaving] = useState(false);

  // Formulario de creación: antes la key se creaba de golpe, sin nombre y con
  // ambos scopes fijos, así que no había forma de saber de qué servicio era cada
  // una ni de dar permisos mínimos (p. ej. solo lectura para el panel).
  const [showForm, setShowForm] = useState(false);
  const [name, setName] = useState("");
  const [send, setSend] = useState(true);
  const [read, setRead] = useState(true);

  const scopes = [...(send ? [SCOPE_SEND] : []), ...(read ? [SCOPE_READ] : [])];

  async function create() {
    // Guarda anti doble-submit: sin ella, un doble clic crea DOS keys y solo se
    // muestra el secret de la última — la otra queda huérfana e irrecuperable.
    if (saving || scopes.length === 0) return;
    setActErr(null);
    setSaving(true);
    try {
      const res = await post<{ keyId: string; secret: string }>(`/admin/applications/${appId}/api-keys`, {
        name: name.trim(),
        scopes,
      });
      setSecret(res);
      setCopied(false);
      setShowForm(false);
      setName("");
      setSend(true);
      setRead(true);
      reload();
    } catch (err) {
      setActErr(err);
    } finally {
      setSaving(false);
    }
  }

  async function revoke(id: string) {
    setActErr(null);
    try {
      await del(`/admin/applications/${appId}/api-keys/${id}`);
      reload();
    } catch (err) {
      setActErr(err);
    }
  }

  // Borrado real: la revocación solo desactiva, y las revocadas se acumulaban en
  // la lista sin poder limpiarlas. El backend solo deja borrar las ya revocadas.
  async function purge(id: string) {
    setActErr(null);
    try {
      await del(`/admin/applications/${appId}/api-keys/${id}/purge`);
      reload();
    } catch (err) {
      setActErr(err);
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

  const keys = data?.data ?? [];
  const hasKeys = keys.length > 0;

  const form = (
    <div className="mb-4 space-y-3 rounded-lg border border-border bg-muted/30 p-3">
      <div className="space-y-1.5">
        <Label htmlFor="key-name">Nombre</Label>
        <Input
          id="key-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="qb-back, qb-panel, qb-booking…"
        />
        <p className="text-xs text-muted-foreground">Para saber de qué servicio es esta key.</p>
      </div>

      <div className="space-y-1.5">
        <Label>Permisos</Label>
        <div className="flex flex-col gap-1.5">
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={send} onChange={(e) => setSend(e.target.checked)} />
            <code className="text-xs">{SCOPE_SEND}</code>
            <span className="text-xs text-muted-foreground">— enviar notificaciones (backends emisores)</span>
          </label>
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={read} onChange={(e) => setRead(e.target.checked)} />
            <code className="text-xs">{SCOPE_READ}</code>
            <span className="text-xs text-muted-foreground">— leer la bandeja (frontends con campana)</span>
          </label>
        </div>
        {scopes.length === 0 && <p className="text-xs text-destructive">Elige al menos un permiso.</p>}
      </div>

      <div className="flex gap-2">
        <Button onClick={create} disabled={saving || scopes.length === 0}>
          {saving ? "Creando…" : "Crear"}
        </Button>
        <Button variant="outline" onClick={() => setShowForm(false)} disabled={saving}>
          Cancelar
        </Button>
      </div>
    </div>
  );

  return (
    <Card
      title="API Keys"
      actions={
        hasKeys && !showForm ? (
          <Button onClick={() => setShowForm(true)} className="max-md:w-9 max-md:px-0">
            <Plus />
            <span className="sr-only md:not-sr-only">Crear API key</span>
          </Button>
        ) : undefined
      }
    >
      <p className="-mt-1 mb-3 text-sm text-muted-foreground">
        Llaves con las que el backend de esta aplicación se autentica (firma HMAC) para enviar notificaciones. El{" "}
        <span className="font-medium text-foreground">secret se muestra una sola vez</span> al crearla.
      </p>

      {showForm && form}

      {secret && (
        <div className="mb-4 rounded-lg border border-amber-300 bg-amber-50 p-3 text-sm">
          <p className="font-medium text-amber-800">Guarda el secret ahora — no se vuelve a mostrar.</p>
          <div className="mt-2 flex items-center gap-2">
            <code className="flex-1 break-all rounded bg-card px-2 py-1 font-mono text-xs">
              {secret.keyId} / {secret.secret}
            </code>
            <Button variant="outline" size="sm" onClick={() => copy(`${secret.keyId} / ${secret.secret}`)}>
              {copied ? <Check /> : <Copy />}
              {copied ? "Copiado" : "Copiar"}
            </Button>
          </div>
        </div>
      )}

      <ErrorText error={actErr} />
      {loading && <p className="text-sm text-muted-foreground">Cargando…</p>}
      <ErrorText error={error} />

      {data && !hasKeys && !loading && !showForm && (
        <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed border-border py-10 text-center">
          <span className="flex h-12 w-12 items-center justify-center rounded-full bg-muted text-muted-foreground">
            <KeyRound className="size-5" />
          </span>
          <div className="space-y-1">
            <p className="text-sm font-medium text-foreground">Aún no hay API keys</p>
            <p className="text-sm text-muted-foreground">
              Crea la primera para que el backend de esta aplicación pueda enviar notificaciones.
            </p>
          </div>
          <Button onClick={() => setShowForm(true)}>
            <Plus />
            Crear la primera API key
          </Button>
        </div>
      )}

      {hasKeys && (
        <Table head={["Nombre", "Key ID", "Scopes", "Estado", "Último uso", ""]}>
          {keys.map((k) => (
            <tr key={k.id} className="border-b border-border/60">
              <td className="rt-pad-action py-2 pr-4 text-sm font-medium text-foreground">
                {k.name || <span className="text-muted-foreground">—</span>}
              </td>
              <td className="py-2 pr-4 font-mono text-xs">{k.keyId}</td>
              <td className="py-2 pr-4">
                {k.scopes.length ? (
                  <div className="flex flex-wrap gap-1">
                    {k.scopes.map((s) => (
                      <span key={s} className="rounded bg-muted px-1.5 py-0.5 font-mono text-[11px] text-muted-foreground">
                        {s}
                      </span>
                    ))}
                  </div>
                ) : (
                  <span className="text-xs text-muted-foreground">sin permisos</span>
                )}
              </td>
              <td className="rt-half py-2 pr-4">
                <Badge value={k.status} />
              </td>
              <td className="rt-half py-2 pr-4 text-muted-foreground">{k.lastUsedAt?.slice(0, 10) ?? "—"}</td>
              <td className="rt-action py-2 pr-4">
                {k.status === "ACTIVE" && (
                  <ConfirmButton message={`¿Revocar la API key ${k.keyId}?`} onConfirm={() => revoke(k.id)}>
                    Revocar
                  </ConfirmButton>
                )}
                {k.status === "REVOKED" && (
                  <ConfirmButton
                    message={`¿Borrar definitivamente la API key ${k.keyId}? Ya está revocada.`}
                    onConfirm={() => purge(k.id)}
                  >
                    Eliminar
                  </ConfirmButton>
                )}
              </td>
            </tr>
          ))}
        </Table>
      )}
    </Card>
  );
}
