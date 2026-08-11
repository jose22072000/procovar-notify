import { get } from "../../api/client";
import { Card, ErrorText, useAsync } from "../../ui";

interface Metrics {
  byStatus: Record<string, number>;
  total: number;
  errorRate: number;
}

// Etiqueta + color por estado de notificación.
const STATUS_META: { key: string; label: string; color: string }[] = [
  { key: "PENDING", label: "Pendientes", color: "text-amber-600" },
  { key: "QUEUED", label: "En cola", color: "text-sky-600" },
  { key: "PROCESSING", label: "Procesando", color: "text-sky-600" },
  { key: "SENT", label: "Enviadas", color: "text-emerald-600" },
  { key: "DELIVERED", label: "Entregadas", color: "text-emerald-600" },
  { key: "READ", label: "Leídas", color: "text-emerald-600" },
  { key: "FAILED", label: "Fallidas", color: "text-destructive" },
  { key: "CANCELLED", label: "Canceladas", color: "text-muted-foreground" },
];

function Kpi({ label, value, color }: { label: string; value: string | number; color?: string }) {
  return (
    <div className="rounded-lg border border-border bg-card p-4">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className={`mt-1 text-2xl font-bold ${color ?? "text-foreground"}`}>{value}</p>
    </div>
  );
}

export default function MetricsTab({ appId }: { appId: string }) {
  const { data, error, loading } = useAsync(() => get<Metrics>(`/admin/applications/${appId}/metrics`), [appId]);

  const er = data?.errorRate ?? 0;
  const erColor = er > 0.05 ? "text-destructive" : er > 0 ? "text-amber-600" : "text-emerald-600";
  // byStatus puede faltar si el backend rompe el contrato: se accede siempre con
  // `?.` para no reventar el render (sin ErrorBoundary blanquearía la SPA).
  const sent = data ? (data.byStatus?.SENT ?? 0) + (data.byStatus?.DELIVERED ?? 0) + (data.byStatus?.READ ?? 0) : 0;
  const failed = data?.byStatus?.FAILED ?? 0;

  return (
    <Card title="Métricas">
      <p className="-mt-1 mb-3 text-sm text-muted-foreground">Resumen de las notificaciones de esta aplicación.</p>
      {loading && <p className="text-sm text-muted-foreground">Cargando…</p>}
      <ErrorText error={error} />

      {data && (
        <div className="space-y-5">
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <Kpi label="Total" value={data.total} />
            <Kpi label="Enviadas" value={sent} color="text-emerald-600" />
            <Kpi label="Fallidas" value={failed} color={failed > 0 ? "text-destructive" : undefined} />
            <Kpi label="Tasa de error" value={`${(er * 100).toFixed(1)}%`} color={erColor} />
          </div>

          <div>
            <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">Desglose por estado</p>
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
              {STATUS_META.map((s) => (
                <div key={s.key} className="rounded-lg border border-border bg-card p-3 text-center">
                  <div className={`text-2xl font-bold ${s.color}`}>{data.byStatus?.[s.key] ?? 0}</div>
                  <div className="mt-0.5 text-xs text-muted-foreground">{s.label}</div>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}
    </Card>
  );
}
