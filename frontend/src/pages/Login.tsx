import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { Loader2, LogIn, AlertTriangle } from "lucide-react";
import { API_BASE } from "@/api/client";
import { Button } from "@/components/ui/button";

/**
 * Entrada a Avisos.
 *
 * Avisos ya NO tiene login propio. Quien administra es quien diga
 * procovar-auth, con el permiso `avisos.entrar` — una sola lista de gente para
 * todo Procovar, y una sola baja el día que alguien se va.
 *
 * Por eso esta pantalla no pide nada: manda al hub y vuelve. Solo se para a
 * enseñar algo cuando el viaje falla, para no dejar a nadie delante de una
 * pantalla en blanco sin saber qué pasó.
 */

const MOTIVOS: Record<string, string> = {
  "no-disponible": "No se pudo contactar con Procovar. Inténtalo en un momento.",
  "sin-codigo": "La vuelta desde Procovar llegó incompleta. Prueba otra vez.",
  "canje-fallido": "Procovar no reconoció la entrada. Prueba otra vez.",
  "sin-permiso": "Tu cuenta de Procovar no tiene acceso a Avisos. Habla con quien administre los permisos.",
};

export default function Login() {
  const [params] = useSearchParams();
  const motivo = params.get("sso");
  const destino = `${API_BASE}/admin/auth/sso/login`;
  const [yendo, setYendo] = useState(!motivo);

  // Sin motivo de error, se va directo: nadie tiene por qué dar un clic de más
  // para hacer lo único que se puede hacer aquí.
  useEffect(() => {
    if (motivo) return;
    window.location.replace(destino);
  }, [motivo, destino]);

  function reintentar() {
    setYendo(true);
    window.location.replace(destino);
  }

  // Si se va al hub, no se pinta NADA. Enseñar la caja de Avisos medio segundo
  // antes de saltar hace pensar que la aplicación se equivocó y se corrigió
  // sola. Solo hay pantalla cuando de verdad hay algo que contar (un error).
  if (!motivo) return null;

  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <div className="w-full max-w-sm space-y-6 border border-border p-8">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold">Avisos</h1>
          <p className="text-sm text-muted-foreground">
            Se entra con tu cuenta de Procovar.
          </p>
        </div>

        {motivo && (
          <div
            role="alert"
            className="flex gap-2 border border-destructive/50 p-3 text-sm text-destructive"
          >
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden />
            <span>{MOTIVOS[motivo] ?? "No se pudo entrar. Inténtalo otra vez."}</span>
          </div>
        )}

        <Button className="w-full" onClick={reintentar} disabled={yendo}>
          {yendo ? (
            <>
              <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden />
              Yendo a Procovar…
            </>
          ) : (
            <>
              <LogIn className="mr-2 h-4 w-4" aria-hidden />
              Entrar con Procovar
            </>
          )}
        </Button>
      </div>
    </div>
  );
}
