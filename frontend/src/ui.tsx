// Capa de componentes de la app compuesta sobre shadcn/ui (web/src/components/ui)
// + tokens de diseño. Mantiene una API cómoda (Card con título, Table con head,
// Badge por estado, ConfirmButton, useAsync) usada por todas las pantallas.
import { Children, cloneElement, isValidElement, useCallback, useEffect, useState, type ReactElement, type ReactNode } from "react";
import { Trash2 } from "lucide-react";
import { ApiError } from "./api/client";
import { cn } from "@/lib/utils";
import { Button as ShButton } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea as ShTextarea } from "@/components/ui/textarea";
import {
  Select as ShSelect,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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

// Button: mapea las variantes históricas (primary/secondary/danger) a shadcn.
const VARIANT_MAP = { primary: "default", secondary: "secondary", danger: "destructive" } as const;

export function Button({
  children,
  onClick,
  variant = "primary",
  type = "button",
  disabled,
}: {
  children: ReactNode;
  onClick?: () => void;
  variant?: "primary" | "secondary" | "danger";
  type?: "button" | "submit";
  disabled?: boolean;
}) {
  return (
    <ShButton type={type} onClick={onClick} disabled={disabled} variant={VARIANT_MAP[variant]}>
      {children}
    </ShButton>
  );
}

export { Input, Label };

// Select de shadcn (Radix) con la API cómoda de un <select> nativo: se le pasan
// <option> hijos y un onChange estilo evento. El dropdown es el de shadcn (no el
// del navegador). El value vacío se mapea a un centinela (Radix no admite "").
const EMPTY_VALUE = "__empty__";

export function Select({
  value,
  onChange,
  children,
  disabled,
  className,
  id,
}: {
  value?: string;
  onChange?: (e: { target: { value: string } }) => void;
  children?: ReactNode;
  disabled?: boolean;
  className?: string;
  id?: string;
}) {
  const options: { value: string; label: ReactNode }[] = [];
  Children.forEach(children, (child) => {
    if (!isValidElement(child) || child.type !== "option") return;
    const p = child.props as { value?: string | number; children?: ReactNode };
    const label = p.children;
    const val = p.value !== undefined ? String(p.value) : String(label ?? "");
    options.push({ value: val, label });
  });
  const toItem = (v: string) => (v === "" ? EMPTY_VALUE : v);

  return (
    <ShSelect
      value={value !== undefined ? toItem(value) : undefined}
      onValueChange={(v) => onChange?.({ target: { value: v === EMPTY_VALUE ? "" : v } })}
      disabled={disabled}
    >
      <SelectTrigger id={id} className={className}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map((o, i) => (
          <SelectItem key={`${o.value}-${i}`} value={toItem(o.value)}>
            {o.label}
          </SelectItem>
        ))}
      </SelectContent>
    </ShSelect>
  );
}

// Textarea: usa el componente shadcn pero conserva monoespaciado (editores JSON).
export function Textarea(props: React.TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return <ShTextarea {...props} className={cn("font-mono text-xs", props.className)} />;
}

// Badge de estado: en < 768px se muestra como palabra en minúscula con color
// (sin píldora); a partir de 768px vuelve a ser una pastilla. El tono deriva del valor.
const BADGE_TONE: Record<string, { text: string; pill: string }> = {
  ok: { text: "text-primary", pill: "md:bg-primary/10" },
  bad: { text: "text-destructive", pill: "md:bg-destructive/10" },
  warn: { text: "text-amber-600 dark:text-amber-400", pill: "md:bg-amber-500/15" },
  muted: { text: "text-muted-foreground", pill: "md:bg-secondary md:text-secondary-foreground" },
};

// statusTone clasifica un valor de estado en su tono (criterio único, reusado
// por Badge y por el texto coloreado de estado en otras vistas).
export function statusTone(value: string): keyof typeof BADGE_TONE {
  if (/^(ACTIVE|SENT|COMPLETED|DELIVERED|sí|true)$/i.test(value)) return "ok";
  if (/FAIL|DISABLED|REVOKED|CANCELLED/i.test(value)) return "bad";
  if (/PENDING|QUEUED|RETRY|SCHEDULED/i.test(value)) return "warn";
  return "muted";
}

// statusTextColor devuelve la clase de color de texto del estado (sin píldora).
export function statusTextColor(value: string): string {
  return BADGE_TONE[statusTone(value)].text;
}

export function Badge({ value }: { value: string }) {
  const t = BADGE_TONE[statusTone(value)];
  return (
    <span
      className={cn(
        "inline-flex items-center font-medium lowercase",
        t.text,
        "md:border md:border-transparent md:px-2 md:py-0.5 md:text-xs md:normal-case",
        t.pill,
      )}
    >
      {value}
    </span>
  );
}

// ConfirmButton: botón destructivo que pide confirmación en un AlertDialog de
// shadcn (no la alerta del navegador).
export function ConfirmButton({ onConfirm, children, message }: { onConfirm: () => void; children: ReactNode; message?: string }) {
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <ShButton
          type="button"
          variant="outline"
          size="sm"
          className="border-destructive/30 text-destructive hover:bg-destructive/10 hover:text-destructive max-lg:w-8 max-lg:px-0"
        >
          <Trash2 />
          <span className="sr-only lg:not-sr-only">{children}</span>
        </ShButton>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{message ?? "¿Confirmar esta acción?"}</AlertDialogTitle>
          <AlertDialogDescription>Esta acción no se puede deshacer.</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancelar</AlertDialogCancel>
          <AlertDialogAction
            onClick={onConfirm}
            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            Confirmar
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

export function Card({ title, children, actions }: { title?: string; children: ReactNode; actions?: ReactNode }) {
  return (
    <div className="rounded-xl border border-border bg-card p-4 text-card-foreground shadow-sm">
      {(title || actions) && (
        <div className="mb-3 flex items-center justify-between">
          {title && <h3 className="text-sm font-semibold text-foreground">{title}</h3>}
          {actions}
        </div>
      )}
      {children}
    </div>
  );
}

export function ErrorText({ error }: { error: unknown }) {
  if (!error) return null;
  const msg = error instanceof ApiError ? error.message : String(error);
  return <p className="text-sm text-destructive">{msg}</p>;
}

export function Table({ head, children }: { head: string[]; children: ReactNode }) {
  // En móvil cada fila se muestra como tarjeta (ver `.responsive-table` en index.css).
  // Etiquetamos cada celda con su encabezado (data-label) para mostrar "Etiqueta: valor".
  const rows = Children.map(children, (row) => {
    if (!isValidElement(row)) return row;
    const el = row as ReactElement<{ children?: ReactNode }>;
    const cells = Children.map(el.props.children, (cell, i) =>
      isValidElement(cell) ? cloneElement(cell as ReactElement<Record<string, unknown>>, { "data-label": head[i] ?? "" }) : cell,
    );
    return cloneElement(el, undefined, cells);
  });

  return (
    <div className="w-full overflow-x-auto">
      <table className="responsive-table w-full text-left text-sm">
        <thead>
          <tr className="border-b border-border text-xs uppercase tracking-wide text-muted-foreground">
            {head.map((h, i) => (
              <th key={h || i} className="py-2 pr-4 font-medium">
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>{rows}</tbody>
      </table>
    </div>
  );
}

// useAsync ejecuta una promesa y expone {data, error, loading, reload}.
export function useAsync<T>(fn: () => Promise<T>, deps: unknown[] = []): {
  data: T | null;
  error: unknown;
  loading: boolean;
  reload: () => void;
} {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [loading, setLoading] = useState(true);
  const [tick, setTick] = useState(0);

  // eslint-disable-next-line react-hooks/exhaustive-deps
  const run = useCallback(fn, deps);

  // Cada ejecución tiene su flag `ignore`: si las deps cambian (o se desmonta)
  // mientras hay una petición en vuelo, su resultado se descarta y no pisa al de
  // la petición nueva — evita la condición de carrera (p. ej. paginar Auditoría).
  useEffect(() => {
    let ignore = false;
    setLoading(true);
    run()
      .then((d) => {
        if (ignore) return;
        setData(d);
        setError(null);
      })
      .catch((e) => {
        if (!ignore) setError(e);
      })
      .finally(() => {
        if (!ignore) setLoading(false);
      });
    return () => {
      ignore = true;
    };
  }, [run, tick]);

  const reload = useCallback(() => setTick((t) => t + 1), []);
  return { data, error, loading, reload };
}
