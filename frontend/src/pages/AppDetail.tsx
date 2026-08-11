import { useState } from "react";
import { useParams, useSearchParams } from "react-router-dom";
import {
  Activity,
  Ban,
  BarChart3,
  Bell,
  CalendarClock,
  FileText,
  Info,
  KeyRound,
  Mail,
  Menu,
  Pencil,
  Route as RouteIcon,
  Save,
  ScrollText,
  Send,
  ShieldCheck,
  Webhook,
  X,
  type LucideIcon,
} from "lucide-react";
import { get, patch, put } from "../api/client";
import { useAuth } from "../auth/auth";
import { Button, ErrorText, Input, Label, Select, statusTextColor, useAsync } from "../ui";
import { Breadcrumb } from "@/components/Breadcrumb";
import { ErrorBoundary } from "@/components/ErrorBoundary";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Button as ShButton } from "@/components/ui/button";
import { cn } from "@/lib/utils";

// Componente de cada pestaña (por su key).
const TAB_COMPONENTS: Record<string, React.ComponentType<{ appId: string }>> = {
  keys: ApiKeys,
  smtp: Smtp,
  providers: Providers,
  routes: RoutesTab,
  templates: Templates,
  notifications: Notifications,
  recurring: Recurring,
  monitor: Monitor,
  metrics: Metrics,
  webhooks: Webhooks,
  suppressions: Suppressions,
  audit: Audit,
  privacy: Privacy,
};
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import ApiKeys from "./tabs/ApiKeys";
import Smtp from "./tabs/Smtp";
import Providers from "./tabs/Providers";
import RoutesTab from "./tabs/Routes";
import Templates from "./tabs/Templates";
import Notifications from "./tabs/Notifications";
import Monitor from "./tabs/Monitor";
import Metrics from "./tabs/Metrics";
import Recurring from "./tabs/Recurring";
import Privacy from "./tabs/Privacy";
import Audit from "./tabs/Audit";
import Webhooks from "./tabs/Webhooks";
import Suppressions from "./tabs/Suppressions";

interface AppInfo {
  id: string;
  name: string;
  slug: string;
  status: string;
  dailySendQuota: number;
  monthlySendQuota: number;
  createdAt: string;
}

// Pestañas agrupadas por sección, con su icono (barra lateral vertical).
const GROUPS: { title: string; tabs: { key: string; label: string; icon: LucideIcon }[] }[] = [
  {
    title: "Configuración",
    tabs: [
      { key: "keys", label: "API Keys", icon: KeyRound },
      { key: "smtp", label: "SMTP", icon: Mail },
      { key: "providers", label: "Proveedores", icon: Send },
      { key: "routes", label: "Tipos de notificación", icon: RouteIcon },
      { key: "templates", label: "Templates", icon: FileText },
    ],
  },
  {
    title: "Actividad",
    tabs: [
      { key: "notifications", label: "Notificaciones", icon: Bell },
      { key: "monitor", label: "Monitor", icon: Activity },
      { key: "metrics", label: "Métricas", icon: BarChart3 },
      { key: "recurring", label: "Recurrentes", icon: CalendarClock },
    ],
  },
  {
    title: "Integraciones",
    tabs: [
      { key: "webhooks", label: "Webhooks", icon: Webhook },
      { key: "suppressions", label: "Supresiones", icon: Ban },
    ],
  },
  {
    title: "Seguridad y datos",
    tabs: [
      { key: "audit", label: "Auditoría", icon: ScrollText },
      { key: "privacy", label: "Privacidad", icon: ShieldCheck },
    ],
  },
];
const ALL_TABS = GROUPS.flatMap((g) => g.tabs);

// Cabecera de la aplicación: resumen del tenant que se está gestionando
// (nombre, slug, estado y cuotas de envío). El super-admin puede editarlo.
function AppHeader({ appId, data, error, reload }: { appId: string; data: AppInfo | null; error: unknown; reload: () => void }) {
  const { admin } = useAuth();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [status, setStatus] = useState("ACTIVE");
  const [daily, setDaily] = useState(0);
  const [monthly, setMonthly] = useState(0);
  const [saving, setSaving] = useState(false);
  const [saveErr, setSaveErr] = useState<unknown>(null);
  const superAdmin = admin?.role === "SUPER_ADMIN";

  // Al abrir el modal, precarga los valores actuales.
  function onOpenChange(v: boolean) {
    if (v && data) {
      setName(data.name);
      setSlug(data.slug);
      setStatus(data.status);
      setDaily(data.dailySendQuota);
      setMonthly(data.monthlySendQuota);
      setSaveErr(null);
    }
    setOpen(v);
  }

  async function save() {
    setSaving(true);
    setSaveErr(null);
    try {
      // Dos escrituras secuenciales (no atómicas: son endpoints distintos). Si el
      // patch va bien y el put falla, la primera ya se persistió; por eso el
      // reload() del finally re-sincroniza la cabecera con la BD pase lo que pase,
      // evitando que muestre datos que ya no coinciden con lo guardado.
      await patch(`/admin/applications/${appId}`, { name, slug, status });
      await put(`/admin/applications/${appId}/quota`, { dailySendQuota: Number(daily), monthlySendQuota: Number(monthly) });
      setOpen(false);
    } catch (err) {
      setSaveErr(err);
    } finally {
      setSaving(false);
      reload();
    }
  }

  // 0 = sin tope de envíos; lo mostramos como texto, no como "∞".
  const quotaLabel = (n: number) => (n > 0 ? `${n.toLocaleString()}/envíos` : "Sin límite");

  return (
    <div className="rounded-xl border border-border bg-card p-4 shadow-sm sm:p-5">
      <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Aplicación</p>
      <ErrorText error={error} />

      {data && (
        <>
          <div className="mt-1 flex items-start justify-between gap-3">
            <div className="flex min-w-0 items-center gap-2.5">
              <span className="flex h-9 w-9 shrink-0 items-center justify-center bg-primary/10 text-sm font-semibold uppercase text-primary sm:h-11 sm:w-11 sm:text-base">
                {data.name.slice(0, 1)}
              </span>
              <div className="min-w-0">
                <h2 className="truncate text-lg font-semibold tracking-tight sm:text-xl">{data.name}</h2>
                <div className="mt-0.5 flex items-center gap-1.5 text-xs text-muted-foreground sm:text-sm">
                  <span className="min-w-0 truncate font-mono">{data.slug}</span>
                  <span className="shrink-0">·</span>
                  <span className={cn("shrink-0 font-medium", statusTextColor(data.status))}>{data.status?.toLowerCase() ?? "—"}</span>
                </div>
              </div>
            </div>

            {superAdmin && (
              <Dialog open={open} onOpenChange={onOpenChange}>
                <DialogTrigger asChild>
                  <ShButton className="shrink-0 max-sm:w-9 max-sm:px-0">
                    <Pencil />
                    <span className="sr-only sm:not-sr-only">Editar</span>
                  </ShButton>
                </DialogTrigger>
                <DialogContent>
                  <DialogHeader>
                    <DialogTitle>Editar aplicación</DialogTitle>
                    <DialogDescription>Nombre, estado y cuotas de envío de «{data.name}».</DialogDescription>
                  </DialogHeader>

                  <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                    <div className="space-y-1.5 sm:col-span-2">
                      <Label htmlFor="app-name-edit">Nombre</Label>
                      <Input id="app-name-edit" value={name} onChange={(e) => setName(e.target.value)} />
                    </div>
                    <div className="space-y-1.5 sm:col-span-2">
                      <Label htmlFor="app-slug-edit">Slug</Label>
                      <Input id="app-slug-edit" value={slug} onChange={(e) => setSlug(e.target.value)} />
                      <p className="text-xs text-muted-foreground">
                        Identificador único del tenant. Cambiarlo es seguro: las API keys autentican por
                        <code className="mx-1">key_id</code>→aplicación, nunca por el slug.
                      </p>
                    </div>
                    <div className="space-y-1.5 sm:col-span-2">
                      <Label htmlFor="app-status-edit">Estado</Label>
                      <Select id="app-status-edit" value={status} onChange={(e) => setStatus(e.target.value)}>
                        <option value="ACTIVE">Activa</option>
                        <option value="DISABLED">Deshabilitada</option>
                      </Select>
                    </div>
                    <div className="space-y-1.5">
                      <Label htmlFor="app-daily">Cuota diaria</Label>
                      <Input id="app-daily" type="number" min={0} value={daily} onChange={(e) => setDaily(Number(e.target.value))} />
                      <p className="text-xs text-muted-foreground">0 = sin límite.</p>
                    </div>
                    <div className="space-y-1.5">
                      <Label htmlFor="app-monthly">Cuota mensual</Label>
                      <Input id="app-monthly" type="number" min={0} value={monthly} onChange={(e) => setMonthly(Number(e.target.value))} />
                      <p className="text-xs text-muted-foreground">0 = sin límite.</p>
                    </div>
                  </div>

                  <ErrorText error={saveErr} />

                  <DialogFooter>
                    <Button variant="secondary" onClick={() => setOpen(false)}>
                      <X />
                      Cancelar
                    </Button>
                    <Button onClick={save} disabled={saving}>
                      <Save />
                      {saving ? "Guardando…" : "Guardar"}
                    </Button>
                  </DialogFooter>
                </DialogContent>
              </Dialog>
            )}
          </div>

          <div className="mt-4 grid max-w-md grid-cols-2 gap-3">
            <div className="rounded-lg border border-border bg-muted/40 p-3">
              <p className="text-xs text-muted-foreground">Cuota diaria</p>
              <p className="text-sm font-semibold text-foreground">{quotaLabel(data.dailySendQuota)}</p>
            </div>
            <div className="rounded-lg border border-border bg-muted/40 p-3">
              <p className="text-xs text-muted-foreground">Cuota mensual</p>
              <p className="text-sm font-semibold text-foreground">{quotaLabel(data.monthlySendQuota)}</p>
            </div>
          </div>
          <p className="mt-2 flex items-start gap-1.5 text-xs text-muted-foreground">
            <Info className="mt-0.5 size-3.5 shrink-0" />
            Las cuotas limitan cuántas notificaciones puede enviar esta aplicación. Al superarlas, los envíos se
            rechazan hasta el siguiente periodo. «Sin límite» = sin tope.
          </p>
        </>
      )}
    </div>
  );
}

/** Lista de pestañas agrupada. Se reutiliza en la sidebar de escritorio y en el drawer móvil. */
function TabNav({ className, onSelect }: { className?: string; onSelect?: () => void }) {
  return (
    <TabsList className={cn("h-auto flex-col items-stretch gap-0.5 rounded-xl border border-border bg-card p-2", className)}>
      {GROUPS.map((g) => (
        <div key={g.title} className="mb-1 last:mb-0">
          <p className="px-2 pb-1 pt-2 text-xs font-bold uppercase tracking-wide text-foreground">{g.title}</p>
          {g.tabs.map((t) => {
            const Icon = t.icon;
            return (
              <TabsTrigger
                key={t.key}
                value={t.key}
                onClick={onSelect}
                className="w-full justify-start gap-2 rounded-md px-2 py-1.5 text-muted-foreground shadow-none hover:bg-accent hover:text-foreground data-[state=active]:bg-primary/10 data-[state=active]:text-primary data-[state=active]:shadow-none"
              >
                <Icon className="size-4 shrink-0" />
                {t.label}
              </TabsTrigger>
            );
          })}
        </div>
      ))}
    </TabsList>
  );
}

export default function AppDetail() {
  const { appId = "" } = useParams();
  const [params, setParams] = useSearchParams();
  // La pestaña activa vive en la URL (?tab=…): se puede abrir una pestaña concreta
  // por enlace (p. ej. al volver del editor) y, al escribirla también al cambiar,
  // persiste tras recargar la página.
  const tab = ALL_TABS.some((t) => t.key === params.get("tab")) ? (params.get("tab") as string) : "keys";
  const setTab = (key: string) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        next.set("tab", key);
        return next;
      },
      { replace: true },
    );
  const [navOpen, setNavOpen] = useState(false);
  const current = ALL_TABS.find((t) => t.key === tab);
  // Un único fetch de la app: lo comparten el breadcrumb y la cabecera (antes
  // se pedía dos veces el mismo recurso). L17.
  const { data: appInfo, error: appErr, reload: reloadApp } = useAsync(() => get<AppInfo>(`/admin/applications/${appId}`), [appId]);

  return (
    <div className="space-y-6">
      <Breadcrumb items={[{ label: "Aplicaciones", to: "/apps" }, { label: appInfo?.name ?? "…" }]} />
      <AppHeader appId={appId} data={appInfo ?? null} error={appErr} reload={reloadApp} />

      <Tabs value={tab} onValueChange={setTab} orientation="vertical" className="flex flex-col gap-4 md:flex-row md:items-start">
        {/* Móvil: botón que abre el menú lateral */}
        <button
          type="button"
          onClick={() => setNavOpen(true)}
          aria-label="Abrir menú de secciones"
          className="flex w-full items-center gap-2 rounded-md border border-input bg-card px-3 py-2 text-sm font-medium shadow-sm transition-colors hover:bg-accent md:hidden"
        >
          <Menu className="size-4 shrink-0" />
          {current ? <current.icon className="size-4 shrink-0 text-primary" /> : null}
          <span className="truncate">{current?.label ?? "Secciones"}</span>
        </button>

        {/* Escritorio: sidebar fija */}
        <TabNav className="hidden w-56 shrink-0 md:sticky md:top-4 md:flex" />

        {/* Móvil: menú lateral deslizable */}
        {navOpen && (
          <div className="fixed inset-0 z-50 md:hidden" role="dialog" aria-modal="true">
            <div className="absolute inset-0 bg-black/40" onClick={() => setNavOpen(false)} />
            <div className="absolute inset-y-0 left-0 flex w-72 max-w-[80vw] flex-col overflow-y-auto border-r border-border bg-card shadow-xl">
              <div className="flex items-center justify-between border-b border-border px-3 py-2.5">
                <span className="text-sm font-semibold text-foreground">Secciones</span>
                <button
                  type="button"
                  onClick={() => setNavOpen(false)}
                  aria-label="Cerrar menú"
                  className="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                >
                  <X className="size-4" />
                </button>
              </div>
              <TabNav className="border-0 bg-transparent p-2" onSelect={() => setNavOpen(false)} />
            </div>
          </div>
        )}

        <div className="min-w-0 flex-1">
          {ALL_TABS.map((t) => {
            const C = TAB_COMPONENTS[t.key];
            return (
              <TabsContent key={t.key} value={t.key} className="mt-0 min-h-[55vh]">
                {/* Aísla el fallo de una pestaña: el resto del panel sigue vivo. */}
                {C ? (
                  <ErrorBoundary>
                    <C appId={appId} />
                  </ErrorBoundary>
                ) : null}
              </TabsContent>
            );
          })}
        </div>
      </Tabs>
    </div>
  );
}
