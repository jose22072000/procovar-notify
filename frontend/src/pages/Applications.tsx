import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { Plus, Settings2, X } from "lucide-react";
import { get, post } from "../api/client";
import { Badge, Card, ErrorText, useAsync } from "../ui";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

interface App {
  id: string;
  name: string;
  slug: string;
  status: string;
  createdAt: string;
}

export default function Applications() {
  const { data, error, loading, reload } = useAsync(() => get<{ data: App[] }>("/admin/applications"));
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [createErr, setCreateErr] = useState<unknown>(null);
  const [saving, setSaving] = useState(false);

  function openNew() {
    setName("");
    setSlug("");
    setCreateErr(null);
    setOpen(true);
  }

  async function create(e: React.FormEvent) {
    e.preventDefault();
    if (saving) return; // evita doble-submit
    setCreateErr(null);
    setSaving(true);
    try {
      await post("/admin/applications", { name, slug });
      setOpen(false);
      setName("");
      setSlug("");
      reload();
    } catch (err) {
      setCreateErr(err);
    } finally {
      setSaving(false);
    }
  }

  const apps = data?.data ?? [];

  return (
    <div className="space-y-6">
      {/* Cabecera: el botón va debajo a ancho completo en móvil (≤425) y al extremo
          derecho alineado con el título en pantallas mayores (≥426). */}
      <div className="flex flex-col gap-3 min-[426px]:flex-row min-[426px]:items-center min-[426px]:justify-between">
        <div>
          <h2 className="text-xl font-semibold tracking-tight">Aplicaciones</h2>
          <p className="text-sm text-muted-foreground">Gestiona los tenants de la plataforma.</p>
        </div>
        <Button className="w-full min-[426px]:w-auto" onClick={openNew}>
          <Plus />
          Nueva aplicación
        </Button>
      </div>

      <Card title={apps.length === 1 ? "1 aplicación" : `${apps.length} aplicaciones`}>
        {loading && <p className="text-sm text-muted-foreground">Cargando…</p>}
        <ErrorText error={error} />
        {data && apps.length === 0 && (
          <p className="text-sm text-muted-foreground">Aún no hay aplicaciones. Crea la primera con «Nueva aplicación».</p>
        )}

        {/* Tarjetas: 320–499px (la tabla no se lee bien en pantallas tan estrechas). */}
        {apps.length > 0 && (
          <div className="space-y-2 min-[500px]:hidden">
            {apps.map((a) => (
              <Link
                key={a.id}
                to={`/apps/${a.id}`}
                className="flex items-center gap-3 border border-border bg-card p-3 transition-colors hover:bg-muted/60"
              >
                <span className="flex h-10 w-10 shrink-0 items-center justify-center bg-primary/10 text-sm font-semibold uppercase text-primary">
                  {a.name.slice(0, 1)}
                </span>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium text-foreground">{a.name}</p>
                  <p className="truncate font-mono text-xs text-muted-foreground">{a.slug}</p>
                </div>
                <div className="flex shrink-0 flex-col items-end gap-1">
                  <Badge value={a.status} />
                  <span className="text-[11px] text-muted-foreground">{a.createdAt?.slice(0, 10) ?? "—"}</span>
                </div>
              </Link>
            ))}
          </div>
        )}

        {/* Tabla: ≥500px. */}
        {apps.length > 0 && (
          <div className="hidden overflow-x-auto min-[500px]:block">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-border text-xs uppercase tracking-wide text-muted-foreground">
                  <th className="py-2 pr-4 font-medium">Aplicación</th>
                  <th className="py-2 pr-4 font-medium">Estado</th>
                  <th className="py-2 pr-4 font-medium">Creada</th>
                  <th className="py-2 font-medium"></th>
                </tr>
              </thead>
              <tbody>
                {apps.map((a) => (
                  <tr
                    key={a.id}
                    onClick={() => navigate(`/apps/${a.id}`)}
                    className="group cursor-pointer border-b border-border/60 transition-colors hover:bg-muted/60"
                  >
                    <td className="py-2 pr-4">
                      <div className="flex items-center gap-3">
                        <span className="flex h-9 w-9 shrink-0 items-center justify-center bg-primary/10 text-sm font-semibold uppercase text-primary">
                          {a.name.slice(0, 1)}
                        </span>
                        <div className="min-w-0">
                          <p className="truncate font-medium text-foreground">{a.name}</p>
                          <p className="truncate font-mono text-xs text-muted-foreground">{a.slug}</p>
                        </div>
                      </div>
                    </td>
                    <td className="py-2 pr-4">
                      <Badge value={a.status} />
                    </td>
                    <td className="py-2 pr-4 text-muted-foreground">{a.createdAt?.slice(0, 10) ?? "—"}</td>
                    <td className="py-2 text-right">
                      <Button
                        asChild
                        variant="outline"
                        size="sm"
                        className="max-lg:w-8 max-lg:px-0 group-hover:border-primary/40 group-hover:bg-primary/5 group-hover:text-primary"
                      >
                        <Link
                          to={`/apps/${a.id}`}
                          aria-label={`Gestionar ${a.name}`}
                          onClick={(e) => e.stopPropagation()}
                        >
                          <Settings2 className="transition-transform group-hover:rotate-45" />
                          <span className="sr-only lg:not-sr-only">Gestionar</span>
                        </Link>
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {/* Modal de alta */}
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Nueva aplicación</DialogTitle>
            <DialogDescription>Da de alta un tenant/cliente (nombre y slug).</DialogDescription>
          </DialogHeader>

          <form onSubmit={create} className="space-y-4" autoComplete="off">
            <div className="space-y-1.5">
              <Label htmlFor="app-name">Nombre</Label>
              <Input id="app-name" autoFocus value={name} onChange={(e) => setName(e.target.value)} required />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="app-slug">Slug</Label>
              <Input id="app-slug" value={slug} onChange={(e) => setSlug(e.target.value)} required />
            </div>

            <ErrorText error={createErr} />

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                <X />
                Cancelar
              </Button>
              <Button type="submit" disabled={saving}>
                <Plus />
                {saving ? "Creando…" : "Crear"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
