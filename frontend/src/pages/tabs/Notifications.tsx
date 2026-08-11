import { useEffect, useState } from "react";
import { ChevronDown, Eye, Inbox, RefreshCw, RotateCcw, X } from "lucide-react";
import { get, post } from "../../api/client";
import { Badge, Card, ErrorText, Select, Table, useAsync } from "../../ui";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

interface Notif {
  id: string;
  templateKey: string;
  channel: string;
  status: string;
  queueState: string;
  createdAt: string;
}
interface Attempt {
  attemptNumber: number;
  status: string;
  errorMessage?: string;
  startedAt: string;
}
interface LogEntry {
  event: string;
  message?: string;
  createdAt: string;
}
interface Detail {
  notification: Notif;
  attempts: Attempt[];
  logs: LogEntry[];
}

export default function Notifications({ appId }: { appId: string }) {
  const base = `/admin/applications/${appId}`;
  const [status, setStatus] = useState("");
  const list = useAsync(
    () => get<{ data: Notif[] }>(`${base}/notifications?limit=20${status ? `&status=${status}` : ""}`),
    [appId, status],
  );
  const [detail, setDetail] = useState<Detail | null>(null);
  const [actErr, setActErr] = useState<unknown>(null);
  // Las notificaciones no llegan en tiempo real; se refrescan con el botón, o
  // automáticamente cada 10 s (activado por defecto). Dependemos de list.reload
  // (estable) y no de list, que cambia de identidad en cada render.
  const [auto, setAuto] = useState(true);
  const reload = list.reload;
  useEffect(() => {
    if (!auto) return;
    const id = setInterval(() => reload(), 10_000);
    return () => clearInterval(id);
  }, [auto, reload]);

  async function open(id: string) {
    setActErr(null);
    try {
      setDetail(await get<Detail>(`${base}/notifications/${id}`));
    } catch (err) {
      setActErr(err);
    }
  }
  async function act(id: string, action: "retry" | "cancel") {
    setActErr(null);
    try {
      await post(`${base}/tasks/${id}/${action}`);
      list.reload();
      open(id);
    } catch (err) {
      setActErr(err);
    }
  }

  // Controles de refresco: botón manual + casilla de auto-refresco.
  const refreshControls = (
    <div className="flex items-center gap-2">
      <Button
        variant="outline"
        size="sm"
        onClick={() => reload()}
        disabled={list.loading}
        title="Actualizar"
        aria-label="Actualizar notificaciones"
      >
        <RefreshCw className={cn(list.loading && "animate-spin")} />
        <span className="sr-only sm:not-sr-only">Actualizar</span>
      </Button>
      <label className="flex cursor-pointer items-center gap-1.5 text-xs text-muted-foreground" title="Refrescar automáticamente cada 10 s">
        <input
          type="checkbox"
          className="size-3.5 rounded border-input accent-primary"
          checked={auto}
          onChange={(e) => setAuto(e.target.checked)}
        />
        Auto
      </label>
    </div>
  );

  const filterSelect = (
    <Select
      className="w-auto py-1"
      value={status}
      onChange={(e) => {
        setStatus(e.target.value);
        setDetail(null); // el panel no debe mostrar una notif fuera del nuevo filtro
      }}
    >
      <option value="">Todas</option>
      {["PENDING", "QUEUED", "SENT", "FAILED", "BOUNCED", "CANCELLED"].map((s) => (
        <option key={s}>{s}</option>
      ))}
    </Select>
  );

  // Cabecera de las listas: refresco + filtro.
  const headerActions = (
    <div className="flex items-center gap-2">
      {refreshControls}
      {filterSelect}
    </div>
  );

  const detailBody = (d: Detail) => (
    <div className="space-y-3 text-sm">
      <div className="flex items-center justify-between gap-2">
        <span className="flex items-center gap-2 font-medium">
          <Badge value={d.notification.status} /> · {d.notification.queueState}
        </span>
        <div className="flex shrink-0 gap-2">
          {d.notification.status === "FAILED" && (
            <Button variant="outline" size="sm" onClick={() => act(d.notification.id, "retry")}>
              <RotateCcw />
              Reintentar
            </Button>
          )}
          {["PENDING", "QUEUED"].includes(d.notification.status) && (
            <Button variant="destructive" size="sm" onClick={() => act(d.notification.id, "cancel")}>
              <X />
              Cancelar
            </Button>
          )}
        </div>
      </div>
      <div>
        <p className="text-xs font-semibold uppercase text-muted-foreground">Intentos</p>
        {d.attempts.map((a, i) => (
          <p key={i} className="text-muted-foreground">
            #{a.attemptNumber} · {a.status} {a.errorMessage ? `· ${a.errorMessage}` : ""}
          </p>
        ))}
      </div>
      <div>
        <p className="text-xs font-semibold uppercase text-muted-foreground">Eventos</p>
        {d.logs.map((l, i) => (
          <p key={i} className="text-muted-foreground">
            {l.createdAt?.slice(11, 19) ?? "—"} · {l.event} {l.message ? `· ${l.message}` : ""}
          </p>
        ))}
      </div>
    </div>
  );

  const rows = list.data?.data ?? [];
  const loaded = !!list.data;
  // Distinguimos «aún no hay ninguna» (sin filtro) de «0 con este filtro».
  const emptyNoFilter = loaded && rows.length === 0 && !status;
  const filteredEmpty = loaded && rows.length === 0 && !!status;

  // Mensaje de «no hay resultados con el filtro» (mantiene el filtro para quitarlo).
  const filteredEmptyBody = (
    <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed border-border py-8 text-center">
      <p className="text-sm text-muted-foreground">
        No hay notificaciones con el estado <span className="font-medium text-foreground">{status}</span>.
      </p>
      <Button variant="outline" size="sm" onClick={() => { setStatus(""); setDetail(null); }}>
        <X />
        Quitar filtro
      </Button>
    </div>
  );

  // Sin ninguna notificación y sin filtro: estado vacío a ancho completo, sin
  // cabecera de tabla ni selector (no aportan nada cuando no hay datos).
  if (emptyNoFilter) {
    return (
      <Card title="Notificaciones" actions={refreshControls}>
        <ErrorText error={list.error} />
        <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed border-border py-14 text-center">
          <span className="flex h-12 w-12 items-center justify-center rounded-full bg-muted text-muted-foreground">
            <Inbox className="size-5" />
          </span>
          <div className="space-y-1">
            <p className="text-sm font-medium text-foreground">Aún no hay notificaciones</p>
            <p className="mx-auto max-w-sm text-sm text-muted-foreground">
              Cuando tu backend envíe notificaciones con esta aplicación, aparecerán aquí con su estado, intentos y detalle.
            </p>
          </div>
        </div>
      </Card>
    );
  }

  return (
    <>
      {/* Móvil (< lg): acordeón — cada notificación se expande con su detalle. */}
      <div className="lg:hidden">
        <Card title="Notificaciones" actions={headerActions}>
          <ErrorText error={list.error} />
          <ErrorText error={actErr} />
          {filteredEmpty && filteredEmptyBody}
          {rows.length > 0 && (
            <div className="space-y-2">
              {rows.map((n) => {
                const expanded = detail?.notification.id === n.id;
                return (
                  <div key={n.id} className="border border-border bg-card">
                    <button
                      type="button"
                      onClick={() => (expanded ? setDetail(null) : open(n.id))}
                      aria-expanded={expanded}
                      className="flex w-full items-center gap-2 p-3 text-left transition-colors hover:bg-muted/40"
                    >
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-sm font-medium text-foreground">{n.templateKey}</p>
                        <p className="text-xs text-muted-foreground">{n.channel}</p>
                      </div>
                      <Badge value={n.status} />
                      <ChevronDown className={cn("size-4 shrink-0 text-muted-foreground transition-transform", expanded && "rotate-180")} />
                    </button>
                    {expanded && detail && <div className="border-t border-border p-3">{detailBody(detail)}</div>}
                  </div>
                );
              })}
            </div>
          )}
        </Card>
      </div>

      {/* Desktop (≥ lg): lista + panel de detalle. */}
      <div className="hidden gap-4 lg:grid lg:grid-cols-2">
        <Card title="Notificaciones" actions={headerActions}>
          <ErrorText error={list.error} />
          {filteredEmpty && filteredEmptyBody}
          {rows.length > 0 && (
            <Table head={["Template", "Canal", "Estado", ""]}>
              {rows.map((n) => {
                const selected = detail?.notification.id === n.id;
                return (
                  <tr key={n.id} className={`border-b border-border/60 ${selected ? "bg-muted/50" : ""}`}>
                    <td className="py-2 pr-4 font-medium">{n.templateKey}</td>
                    <td className="py-2 pr-4">{n.channel}</td>
                    <td className="py-2 pr-4">
                      <Badge value={n.status} />
                    </td>
                    <td className="py-2 pr-4 text-right">
                      <Button variant={selected ? "default" : "outline"} size="sm" onClick={() => open(n.id)}>
                        <Eye />
                        Ver
                      </Button>
                    </td>
                  </tr>
                );
              })}
            </Table>
          )}
        </Card>

        <Card title="Detalle">
          <ErrorText error={actErr} />
          {!detail && <p className="text-sm text-muted-foreground">Selecciona una notificación.</p>}
          {detail && detailBody(detail)}
        </Card>
      </div>
    </>
  );
}
