import { useState, type ReactNode } from "react";
import { Plus, Users, X } from "lucide-react";
import { get, post } from "../api/client";
import { Badge, Card, ErrorText, Input, Label, Select, useAsync } from "../ui";
import { Button } from "@/components/ui/button";
import { Breadcrumb } from "@/components/Breadcrumb";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

interface AdminUser {
  id: string;
  email: string;
  role: string;
  applicationId?: string;
  status: string;
}
interface App {
  id: string;
  name: string;
}

const ROLE_LABEL: Record<string, string> = { SUPER_ADMIN: "Super admin", APP_ADMIN: "Admin de aplicación" };
const empty = { email: "", password: "", role: "APP_ADMIN", applicationId: "" };

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

export default function AdminUsers() {
  const list = useAsync(() => get<{ data: AdminUser[] }>("/admin/admin-users"), []);
  const apps = useAsync(() => get<{ data: App[] }>("/admin/applications"), []);
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ ...empty });
  const [saving, setSaving] = useState(false);
  const [formErr, setFormErr] = useState<unknown>(null);

  const appName = (id?: string) => apps.data?.data?.find((a) => a.id === id)?.name ?? "—";

  function openNew() {
    setForm({ ...empty });
    setFormErr(null);
    setOpen(true);
  }

  async function create(e: React.FormEvent) {
    e.preventDefault();
    if (form.role === "APP_ADMIN" && !form.applicationId) return setFormErr("Elige la aplicación del admin.");
    setSaving(true);
    setFormErr(null);
    try {
      const body: Record<string, unknown> = { email: form.email, password: form.password, role: form.role };
      if (form.role === "APP_ADMIN" && form.applicationId) body.applicationId = form.applicationId;
      await post("/admin/admin-users", body);
      setOpen(false);
      list.reload();
    } catch (err) {
      setFormErr(err);
    } finally {
      setSaving(false);
    }
  }

  const users = list.data?.data ?? [];
  const has = users.length > 0;

  return (
    <div>
      <Breadcrumb items={[{ label: "Aplicaciones", to: "/apps" }, { label: "Usuarios admin" }]} />

      <Card
        title="Usuarios admin"
        actions={
          has ? (
            <Button onClick={openNew}>
              <Plus />
              Nuevo
            </Button>
          ) : undefined
        }
      >
        <p className="-mt-1 mb-3 text-sm text-muted-foreground">Administradores del panel. Los super-admin gestionan todo; los admin de aplicación, solo la suya.</p>

        {list.loading && <p className="text-sm text-muted-foreground">Cargando…</p>}
        <ErrorText error={list.error} />
        <ErrorText error={apps.error} />

        {list.data && !has && (
          <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed border-border py-10 text-center">
            <span className="flex h-12 w-12 items-center justify-center rounded-full bg-muted text-muted-foreground">
              <Users className="size-5" />
            </span>
            <p className="text-sm text-muted-foreground">Aún no hay otros administradores.</p>
            <Button onClick={openNew}>
              <Plus />
              Nuevo
            </Button>
          </div>
        )}

        {has && (
          <div className="divide-y divide-border/60">
            {users.map((u) => (
              <div key={u.id} className="flex flex-wrap items-center gap-3 py-3">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="truncate text-sm font-medium text-foreground">{u.email}</span>
                    <Badge value={u.status} />
                  </div>
                  <p className="truncate text-xs text-muted-foreground">
                    {ROLE_LABEL[u.role] ?? u.role} · {u.role === "SUPER_ADMIN" ? "global" : appName(u.applicationId)}
                  </p>
                </div>
              </div>
            ))}
          </div>
        )}
      </Card>

      {/* Modal de alta */}
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Nuevo usuario admin</DialogTitle>
            <DialogDescription>Crea un administrador del panel.</DialogDescription>
          </DialogHeader>

          <form onSubmit={create} className="space-y-4" autoComplete="off">
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div className="sm:col-span-2">
                <Field label="Email" htmlFor="au-email" required>
                  <Input id="au-email" type="email" autoComplete="off" placeholder="admin@empresa.com" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} required />
                </Field>
              </div>
              <div className="sm:col-span-2">
                <Field label="Contraseña" htmlFor="au-pass" required>
                  <Input id="au-pass" type="password" autoComplete="new-password" placeholder="••••••••" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} required />
                </Field>
              </div>
              <Field label="Rol" htmlFor="au-role" required>
                <Select id="au-role" value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value })}>
                  <option value="APP_ADMIN">Admin de aplicación</option>
                  <option value="SUPER_ADMIN">Super admin</option>
                </Select>
              </Field>
              <Field label="Aplicación" htmlFor="au-app">
                <Select id="au-app" value={form.applicationId} disabled={form.role !== "APP_ADMIN"} onChange={(e) => setForm({ ...form, applicationId: e.target.value })}>
                  <option value="">— Elegir aplicación —</option>
                  {apps.data?.data?.map((a) => (
                    <option key={a.id} value={a.id}>
                      {a.name}
                    </option>
                  ))}
                </Select>
              </Field>
            </div>

            <ErrorText error={formErr} />

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                <X />
                Cancelar
              </Button>
              <Button type="submit" disabled={saving}>
                <Plus />
                {saving ? "Creando…" : "Crear usuario"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
