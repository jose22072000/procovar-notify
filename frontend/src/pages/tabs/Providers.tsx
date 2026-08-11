import { useState, type ReactNode } from "react";
import { Plus, Send, X } from "lucide-react";
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

interface Provider {
  id: string;
  channel: string;
  provider: string;
  name: string;
  status: string;
}

// Proveedores disponibles por canal y sus campos de credenciales.
const PROVIDERS_BY_CHANNEL: Record<string, string[]> = {
  PUSH: ["FCM", "APNS"],
  SMS: ["TWILIO"],
};
type CfgField = { key: string; label: string; type?: string; textarea?: boolean; placeholder?: string };
const CONFIG_FIELDS: Record<string, CfgField[]> = {
  FCM: [{ key: "serverKey", label: "Server key", type: "password", placeholder: "AAAA…" }],
  APNS: [
    { key: "teamId", label: "Team ID", placeholder: "TEAM123456" },
    { key: "keyId", label: "Key ID", placeholder: "KEY1234567" },
    { key: "bundleId", label: "Bundle ID", placeholder: "com.tuempresa.app" },
    { key: "privateKey", label: "Clave privada (.p8)", textarea: true, placeholder: "-----BEGIN PRIVATE KEY-----…" },
  ],
  TWILIO: [
    { key: "accountSid", label: "Account SID", placeholder: "AC…" },
    { key: "authToken", label: "Auth token", type: "password" },
    { key: "from", label: "Número remitente (From)", placeholder: "+34600000000" },
  ],
};

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

export default function ProvidersTab({ appId }: { appId: string }) {
  const base = `/admin/applications/${appId}/providers`;
  const { data, error, loading, reload } = useAsync(() => get<{ data: Provider[] }>(base), [appId]);
  const [open, setOpen] = useState(false);
  const [channel, setChannel] = useState("PUSH");
  const [provider, setProvider] = useState("FCM");
  const [name, setName] = useState("");
  const [config, setConfig] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);
  const [formErr, setFormErr] = useState<unknown>(null);
  const [actErr, setActErr] = useState<unknown>(null);

  function openNew() {
    setChannel("PUSH");
    setProvider("FCM");
    setName("");
    setConfig({});
    setFormErr(null);
    setOpen(true);
  }

  function changeChannel(c: string) {
    setChannel(c);
    setProvider(PROVIDERS_BY_CHANNEL[c][0]);
    setConfig({});
  }

  function changeProvider(p: string) {
    setProvider(p);
    setConfig({});
  }

  async function create(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    setFormErr(null);
    try {
      await post(base, { channel, provider, name, config });
      setOpen(false);
      reload();
    } catch (err) {
      setFormErr(err);
    } finally {
      setSaving(false);
    }
  }

  const providers = data?.data ?? [];
  const has = providers.length > 0;

  const delAction = (p: Provider) => (
    <ConfirmButton message={`¿Eliminar el proveedor «${p.name}»?`} onConfirm={() => del(`${base}/${p.id}`).then(reload).catch(setActErr)}>
      Eliminar
    </ConfirmButton>
  );

  return (
    <Card
      title="Proveedores"
      actions={
        has ? (
          <Button onClick={openNew} className="max-md:w-9 max-md:px-0">
            <Plus />
            <span className="sr-only md:not-sr-only">Nuevo</span>
          </Button>
        ) : undefined
      }
    >
      <p className="-mt-1 mb-3 text-sm text-muted-foreground">Credenciales para enviar Push (FCM/APNS) y SMS (Twilio). Se cifran en reposo (AES-256-GCM).</p>

      <ErrorText error={actErr} />
      {loading && <p className="text-sm text-muted-foreground">Cargando…</p>}
      <ErrorText error={error} />

      {data && !has && (
        <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed border-border py-10 text-center">
          <span className="flex h-12 w-12 items-center justify-center rounded-full bg-muted text-muted-foreground">
            <Send className="size-5" />
          </span>
          <div className="space-y-1">
            <p className="text-sm font-medium text-foreground">Aún no hay proveedores</p>
            <p className="text-sm text-muted-foreground">Añade uno para enviar notificaciones Push o SMS.</p>
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
          {providers.map((p) => (
            <div key={p.id} className="border border-border bg-card p-3">
              <div className="min-w-0">
                <div className="flex min-w-0 items-center gap-2">
                  <span className="truncate text-sm font-medium text-foreground">{p.name}</span>
                  <Badge value={p.status} />
                </div>
                <p className="mt-1 text-xs text-muted-foreground">
                  {p.channel} · {p.provider}
                </p>
              </div>
              <div className="mt-2 flex items-center justify-end gap-1">{delAction(p)}</div>
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
                <th className="py-2 pr-4 font-medium">Canal</th>
                <th className="py-2 pr-4 font-medium">Proveedor</th>
                <th className="py-2 pr-4 font-medium">Estado</th>
                <th className="py-2 font-medium"></th>
              </tr>
            </thead>
            <tbody>
              {providers.map((p) => (
                <tr key={p.id} className="border-b border-border/60 align-top">
                  <td className="py-2 pr-4 font-medium text-foreground">{p.name}</td>
                  <td className="py-2 pr-4 text-muted-foreground">{p.channel}</td>
                  <td className="py-2 pr-4 text-muted-foreground">{p.provider}</td>
                  <td className="py-2 pr-4">
                    <Badge value={p.status} />
                  </td>
                  <td className="py-2">
                    <div className="flex items-center justify-end gap-1">{delAction(p)}</div>
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
            <DialogTitle>Nuevo proveedor</DialogTitle>
            <DialogDescription>Credenciales del proveedor de Push o SMS.</DialogDescription>
          </DialogHeader>

          <form onSubmit={create} className="space-y-4" autoComplete="off">
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field label="Canal" htmlFor="pv-channel" required>
                <Select id="pv-channel" value={channel} onChange={(e) => changeChannel(e.target.value)}>
                  <option value="PUSH">PUSH</option>
                  <option value="SMS">SMS</option>
                </Select>
              </Field>
              <Field label="Proveedor" htmlFor="pv-provider" required>
                <Select id="pv-provider" value={provider} onChange={(e) => changeProvider(e.target.value)}>
                  {PROVIDERS_BY_CHANNEL[channel].map((p) => (
                    <option key={p} value={p}>
                      {p}
                    </option>
                  ))}
                </Select>
              </Field>
              <div className="sm:col-span-2">
                <Field label="Nombre" htmlFor="pv-name" required>
                  <Input id="pv-name" autoComplete="off" placeholder="Mi proveedor" value={name} onChange={(e) => setName(e.target.value)} required />
                </Field>
              </div>
            </div>

            <div className="rounded-lg border border-border p-3">
              <p className="mb-2 text-sm font-semibold text-foreground">Credenciales · {provider}</p>
              <div className="space-y-3">
                {(CONFIG_FIELDS[provider] ?? []).map((f) => (
                  <Field key={f.key} label={f.label} htmlFor={`pv-${f.key}`} required>
                    {f.textarea ? (
                      <Textarea
                        id={`pv-${f.key}`}
                        className="h-24"
                        placeholder={f.placeholder}
                        value={config[f.key] ?? ""}
                        onChange={(e) => setConfig({ ...config, [f.key]: e.target.value })}
                      />
                    ) : (
                      <Input
                        id={`pv-${f.key}`}
                        name={`pv-${f.key}`}
                        type={f.type ?? "text"}
                        autoComplete={f.type === "password" ? "new-password" : "off"}
                        placeholder={f.placeholder}
                        value={config[f.key] ?? ""}
                        onChange={(e) => setConfig({ ...config, [f.key]: e.target.value })}
                      />
                    )}
                  </Field>
                ))}
              </div>
            </div>

            <ErrorText error={formErr} />

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                <X />
                Cancelar
              </Button>
              <Button type="submit" disabled={saving}>
                <Plus />
                {saving ? "Creando…" : "Crear proveedor"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </Card>
  );
}
