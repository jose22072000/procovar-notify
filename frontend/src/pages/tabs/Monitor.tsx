import { useEffect, useState } from "react";
import { API_BASE, tokens, tryRefresh } from "../../api/client";
import { Card } from "../../ui";

interface Summary {
  byState: Record<string, number>;
  byPriority: Record<string, number>;
  total: number;
}

// Etiqueta + color de acento por estado de cola.
const STATES: { key: string; label: string; color: string }[] = [
  { key: "pending", label: "Pendientes", color: "text-amber-600" },
  { key: "scheduled", label: "Programadas", color: "text-sky-600" },
  { key: "active", label: "En proceso", color: "text-primary" },
  { key: "retry", label: "Reintentando", color: "text-amber-600" },
  { key: "completed", label: "Completadas", color: "text-emerald-600" },
  { key: "failed", label: "Fallidas", color: "text-destructive" },
  { key: "cancelled", label: "Canceladas", color: "text-muted-foreground" },
];

const PRIORITIES: { key: string; label: string }[] = [
  { key: "URGENT", label: "Urgente" },
  { key: "HIGH", label: "Alta" },
  { key: "NORMAL", label: "Normal" },
  { key: "LOW", label: "Baja" },
];

function Tile({ label, value, color }: { label: string; value: number; color?: string }) {
  return (
    <div className="rounded-lg border border-border bg-card p-3 text-center">
      <div className={`text-2xl font-bold ${color ?? "text-foreground"}`}>{value}</div>
      <div className="mt-0.5 text-xs text-muted-foreground">{label}</div>
    </div>
  );
}

function LiveBadge({ live }: { live: boolean }) {
  return (
    <span className="inline-flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
      {live ? (
        <>
          <span className="relative flex size-2">
            <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75" />
            <span className="relative inline-flex size-2 rounded-full bg-emerald-500" />
          </span>
          <span className="text-emerald-600">En vivo</span>
        </>
      ) : (
        <>
          <span className="size-2 rounded-full bg-muted-foreground/50" />
          Reconectando…
        </>
      )}
    </span>
  );
}

// Monitor consume el SSE de /tasks/stream vía fetch (EventSource no permite
// cabecera Authorization). El servidor cierra cada ~25s; se reconecta solo.
export default function Monitor({ appId }: { appId: string }) {
  const [summary, setSummary] = useState<Summary | null>(null);
  const [live, setLive] = useState(false);

  useEffect(() => {
    const ctrl = new AbortController();
    let stop = false;
    const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

    async function loop() {
      while (!stop) {
        try {
          // OJO: fetch crudo — hay que anteponer API_BASE a mano. Sin él la
          // petición va al origen de la SPA (que sirve el index.html) en vez de
          // al API, y el stream nunca entrega eventos → "Reconectando…" eterno.
          const res = await fetch(`${API_BASE}/admin/applications/${appId}/tasks/stream`, {
            headers: { Authorization: `Bearer ${tokens.access()}` },
            signal: ctrl.signal,
          });
          // Token de acceso caducado (vive 15 min): renuévalo y reconecta. El
          // fetch crudo del SSE no pasa por el interceptor de api(), así que sin
          // esto el stream se quedaría atascado en 401 y el badge en
          // "Reconectando…" para siempre.
          if (res.status === 401) {
            setLive(false);
            if (!(await tryRefresh())) await sleep(5000); // sesión muerta: no martillees
            continue;
          }
          if (!res.ok || !res.body) {
            setLive(false);
            await sleep(2000);
            continue;
          }
          setLive(true);
          const connectedAt = Date.now();
          const reader = res.body.getReader();
          const decoder = new TextDecoder();
          let buf = "";
          while (!stop) {
            const { done, value } = await reader.read();
            // Cierre normal del servidor (~25s): reconecta en caliente sin bajar
            // a "Reconectando…" para no parpadear el badge en el camino feliz.
            if (done) break;
            buf += decoder.decode(value, { stream: true });
            let idx: number;
            while ((idx = buf.indexOf("\n\n")) >= 0) {
              const event = buf.slice(0, idx);
              buf = buf.slice(idx + 2);
              const dataLine = event.split("\n").find((l) => l.startsWith("data:"));
              if (dataLine) {
                try {
                  setSummary(JSON.parse(dataLine.slice(5).trim()));
                } catch {
                  /* ignora líneas no-JSON */
                }
              }
            }
          }
          // Si el stream se cerró casi al instante (proxy mal configurado,
          // keep-alive vacío) no reconectes en caliente: aplica backoff para no
          // caer en un loop cerrado que martillee el servidor.
          if (!stop && Date.now() - connectedAt < 1000) {
            setLive(false);
            await sleep(2000);
          }
        } catch {
          if (stop) return;
          // Backoff ante fallo de conexión: evita un loop de reconexión cerrado
          // que inunde de peticiones si el servidor no responde.
          setLive(false);
          await sleep(2000);
        }
      }
    }
    loop();
    return () => {
      stop = true;
      ctrl.abort();
    };
  }, [appId]);

  return (
    <div className="space-y-4">
      <Card title="Estado de la cola" actions={<LiveBadge live={live} />}>
        <p className="-mt-1 mb-3 text-sm text-muted-foreground">Procesamiento de las notificaciones de esta aplicación, en tiempo real.</p>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4 lg:grid-cols-7">
          {STATES.map((s) => (
            <Tile key={s.key} label={s.label} value={summary?.byState?.[s.key] ?? 0} color={s.color} />
          ))}
        </div>
        <p className="mt-3 text-sm text-muted-foreground">
          Total: <span className="font-semibold text-foreground">{summary?.total ?? 0}</span>
        </p>
      </Card>

      <Card title="Por prioridad">
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          {PRIORITIES.map((p) => (
            <Tile key={p.key} label={p.label} value={summary?.byPriority?.[p.key] ?? 0} />
          ))}
        </div>
      </Card>
    </div>
  );
}
