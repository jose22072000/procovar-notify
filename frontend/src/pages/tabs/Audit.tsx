import { useState } from "react";
import { ChevronLeft, ChevronRight, ScrollText } from "lucide-react";
import { get } from "../../api/client";
import { cn } from "@/lib/utils";
import { Card, ErrorText, Table, useAsync } from "../../ui";
import { Button } from "@/components/ui/button";

interface AuditEntry {
  id: string;
  actorAdminId: string;
  action: string;
  targetType: string;
  targetId?: string;
  details?: unknown;
  ip?: string;
  userAgent?: string;
  createdAt: string;
}

const PAGE = 50;

const RESOURCE: Record<string, string> = {
  application: "Aplicación",
  api_key: "API key",
  smtp: "SMTP",
  provider: "Proveedor",
  route: "Ruta",
  template: "Plantilla",
  admin_user: "Usuario admin",
  recurring: "Programación",
  retention: "Retención",
  user: "Usuario",
  webhook: "Webhook",
  suppression: "Supresión",
};
const VERB: Record<string, string> = {
  create: "creado",
  update: "actualizado",
  delete: "eliminado",
  revoke: "revocada",
  publish: "publicado",
  suspend: "suspendida",
  add: "añadida",
  remove: "quitada",
  forget: "anonimizado",
};

function actionLabel(a: string): string {
  const [res, verb] = a.split(".");
  return `${RESOURCE[res] ?? res} · ${VERB[verb] ?? verb}`;
}
function actionTone(a: string): string {
  const verb = a.split(".")[1] ?? "";
  if (/delete|revoke|remove|forget|suspend/.test(verb)) return "bg-destructive/10 text-destructive";
  if (/create|add|publish/.test(verb)) return "bg-emerald-100 text-emerald-700";
  return "bg-muted text-muted-foreground";
}

// Registro de acciones de administración (AdminAuditLog) de la aplicación.
export default function Audit({ appId }: { appId: string }) {
  const [offset, setOffset] = useState(0);
  const { data, error, loading } = useAsync(
    () => get<{ data: AuditEntry[] }>(`/admin/applications/${appId}/audit?limit=${PAGE}&offset=${offset}`),
    [appId, offset],
  );
  const rows = data?.data ?? [];

  return (
    <Card title="Auditoría">
      <p className="-mt-1 mb-3 text-sm text-muted-foreground">Registro de las acciones realizadas en el panel de esta aplicación (quién, qué y desde dónde).</p>

      {loading && <p className="text-sm text-muted-foreground">Cargando…</p>}
      <ErrorText error={error} />

      {data && rows.length === 0 && offset === 0 && (
        <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed border-border py-10 text-center">
          <span className="flex h-12 w-12 items-center justify-center rounded-full bg-muted text-muted-foreground">
            <ScrollText className="size-5" />
          </span>
          <p className="text-sm text-muted-foreground">Sin acciones registradas todavía.</p>
        </div>
      )}

      {rows.length > 0 && (
        <Table head={["Fecha", "Acción", "Recurso", "Actor", "IP"]}>
          {rows.map((e) => (
            <tr key={e.id} className="border-b border-border/60 align-top">
              <td className="rt-half whitespace-nowrap py-2 pr-4 text-muted-foreground max-md:whitespace-normal">{e.createdAt?.replace("T", " ").slice(0, 19) ?? "—"}</td>
              <td className="rt-half py-2 pr-4">
                <span className={cn("rounded px-1.5 py-0.5 text-xs font-medium", actionTone(e.action))}>{actionLabel(e.action)}</span>
              </td>
              <td className="rt-half py-2 pr-4 text-muted-foreground">
                {RESOURCE[e.targetType] ?? e.targetType}
                {e.targetId ? <span className="font-mono text-xs text-muted-foreground"> · {e.targetId.slice(0, 8)}</span> : null}
              </td>
              <td className="rt-half py-2 pr-4 font-mono text-xs text-muted-foreground">{e.actorAdminId.slice(0, 8)}</td>
              <td className="py-2 pr-4 text-muted-foreground">{e.ip ?? "—"}</td>
            </tr>
          ))}
        </Table>
      )}

      {/* Página vacía tras avanzar (última página estaba llena y la siguiente no
          tiene filas): sin esto el usuario quedaba atrapado sin "Anteriores". */}
      {data && rows.length === 0 && offset > 0 && (
        <p className="py-6 text-center text-sm text-muted-foreground">No hay más registros.</p>
      )}

      {/* El pager se muestra siempre que haya filas O ya se haya avanzado, para
          que "Anteriores" esté disponible incluso en una página vacía. */}
      {(rows.length > 0 || offset > 0) && (
        <div className="mt-4 flex items-center justify-between">
          <Button variant="outline" size="sm" onClick={() => setOffset(Math.max(0, offset - PAGE))} disabled={offset === 0}>
            <ChevronLeft />
            Anteriores
          </Button>
          {rows.length > 0 && <span className="text-xs text-muted-foreground">desde {offset + 1}</span>}
          <Button variant="outline" size="sm" onClick={() => setOffset(offset + PAGE)} disabled={rows.length < PAGE}>
            Siguientes
            <ChevronRight />
          </Button>
        </div>
      )}
    </Card>
  );
}
