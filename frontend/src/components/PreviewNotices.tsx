// Avisos compartidos de la vista previa, reutilizados por el editor visual, el
// HTML avanzado y los canales no-email (antes estaban duplicados).

// PreviewPausedNotice: la previsualización se pausó por una variable a medias.
export function PreviewPausedNotice({ paused }: { paused: boolean }) {
  if (!paused) return null;
  return (
    <p className="mb-3 rounded-md border border-border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
      Vista previa en pausa: hay una variable sin cerrar. Termina de escribirla y se actualizará.
    </p>
  );
}

// MissingVarsNotice: variables requeridas que faltan en los datos de prueba.
export function MissingVarsNotice({ missing }: { missing?: string[] | null }) {
  if (!missing || missing.length === 0) return null;
  // amber = color de advertencia (el tema no define un token "warning").
  return (
    <p className="mb-3 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800">
      Faltan variables: {missing.join(", ")}
    </p>
  );
}
