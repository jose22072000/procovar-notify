import { useState } from "react";
import { CheckCircle2, Save, UserX } from "lucide-react";
import { put, post } from "../../api/client";
import { Card, ErrorText, Input, Label } from "../../ui";
import { Button } from "@/components/ui/button";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";

function SuccessNote({ children }: { children: React.ReactNode }) {
  return (
    <div className="mt-3 flex items-start gap-2 rounded-lg border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-800">
      <CheckCircle2 className="mt-0.5 size-4 shrink-0" />
      <span>{children}</span>
    </div>
  );
}

// Privacidad (PII): política de retención de datos y derecho al olvido.
export default function Privacy({ appId }: { appId: string }) {
  const base = `/admin/applications/${appId}`;
  const [days, setDays] = useState("90");
  const [saving, setSaving] = useState(false);
  const [retMsg, setRetMsg] = useState("");
  const [retErr, setRetErr] = useState<unknown>(null);

  const [userId, setUserId] = useState("");
  const [forgetMsg, setForgetMsg] = useState("");
  const [forgetErr, setForgetErr] = useState<unknown>(null);

  async function saveRetention(e: React.FormEvent) {
    e.preventDefault();
    setRetMsg("");
    setRetErr(null);
    setSaving(true);
    try {
      await put(`${base}/retention`, { retentionDays: Number(days) });
      setRetMsg(`Retención fijada en ${Number(days)} días.`);
    } catch (err) {
      setRetErr(err);
    } finally {
      setSaving(false);
    }
  }

  async function forget() {
    setForgetMsg("");
    setForgetErr(null);
    try {
      const res = await post<{ anonymized: number }>(`${base}/users/${encodeURIComponent(userId)}/forget`);
      setForgetMsg(`Anonimizados ${res.anonymized} registro(s) del usuario «${userId}».`);
      setUserId("");
    } catch (err) {
      setForgetErr(err);
    }
  }

  return (
    <div className="space-y-4">
      <Card title="Política de retención">
        <p className="-mt-1 mb-3 text-sm text-muted-foreground">
          Las notificaciones y sus datos personales más antiguos que este plazo se anonimizan automáticamente.
        </p>
        <form onSubmit={saveRetention} className="flex flex-col gap-3 sm:flex-row sm:items-end">
          <div className="space-y-1.5 sm:w-40">
            <Label htmlFor="ret-days">Días de retención</Label>
            <Input id="ret-days" type="number" min={1} value={days} onChange={(e) => setDays(e.target.value)} required />
          </div>
          <Button type="submit" disabled={saving}>
            <Save />
            {saving ? "Guardando…" : "Guardar"}
          </Button>
        </form>
        {retMsg && <SuccessNote>{retMsg}</SuccessNote>}
        <ErrorText error={retErr} />
      </Card>

      <Card title="Derecho al olvido">
        <p className="-mt-1 mb-3 text-sm text-muted-foreground">
          Anonimiza de forma <span className="font-medium text-foreground">irreversible</span> los datos personales de un
          usuario en todas sus notificaciones (cumplimiento GDPR).
        </p>
        <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
          <div className="flex-1 space-y-1.5">
            <Label htmlFor="forget-user">User ID a anonimizar</Label>
            <Input id="forget-user" placeholder="u_123" value={userId} onChange={(e) => setUserId(e.target.value)} />
          </div>
          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button variant="destructive" disabled={!userId.trim()}>
                <UserX />
                Anonimizar
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>¿Anonimizar a «{userId}»?</AlertDialogTitle>
                <AlertDialogDescription>
                  Se borrarán de forma irreversible los datos personales de este usuario en todas sus notificaciones. Esta
                  acción no se puede deshacer.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Cancelar</AlertDialogCancel>
                <AlertDialogAction onClick={forget} className="bg-destructive text-destructive-foreground hover:bg-destructive/90">
                  Anonimizar
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </div>
        {forgetMsg && <SuccessNote>{forgetMsg}</SuccessNote>}
        <ErrorText error={forgetErr} />
      </Card>
    </div>
  );
}
