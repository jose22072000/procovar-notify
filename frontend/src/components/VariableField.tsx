import { useRef, useState } from "react";
import { Braces, Plus } from "lucide-react";
import { cn } from "@/lib/utils";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";

// VariableField es un input/textarea con un botón «{{ }}» que abre un popover para
// insertar variables Handlebars en la posición del cursor, sin teclear las llaves.
// `used` son las variables ya presentes en la plantilla (para reinsertarlas).
export function VariableField({
  value,
  onChange,
  used,
  multiline,
  placeholder,
  id,
  className,
  rows,
  onBlur,
}: {
  value: string;
  onChange: (v: string) => void;
  used: string[];
  multiline?: boolean;
  placeholder?: string;
  id?: string;
  className?: string;
  rows?: number;
  onBlur?: () => void;
}) {
  const ref = useRef<HTMLInputElement & HTMLTextAreaElement>(null);
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");

  function insert(varName: string) {
    const clean = varName.trim().replace(/[^a-zA-Z0-9_.]/g, "");
    if (!clean) return;
    const token = `{{${clean}}}`;
    const el = ref.current;
    if (!el) {
      onChange(value + token);
    } else {
      const start = el.selectionStart ?? value.length;
      const end = el.selectionEnd ?? value.length;
      onChange(value.slice(0, start) + token + value.slice(end));
      requestAnimationFrame(() => {
        el.focus();
        const pos = start + token.length;
        el.setSelectionRange(pos, pos);
      });
    }
    setName("");
    setOpen(false);
  }

  const control = multiline ? (
    <Textarea
      ref={ref}
      id={id}
      rows={rows}
      placeholder={placeholder}
      value={value}
      onBlur={onBlur}
      onChange={(e) => onChange(e.target.value)}
      className={cn("pr-10", className)}
    />
  ) : (
    <Input
      ref={ref}
      id={id}
      placeholder={placeholder}
      value={value}
      onBlur={onBlur}
      onChange={(e) => onChange(e.target.value)}
      className={cn("pr-10", className)}
    />
  );

  return (
    <div className="relative">
      {control}
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <button
            type="button"
            aria-label="Insertar variable"
            title="Insertar variable"
            className={cn(
              "absolute right-1.5 flex h-6 w-6 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-accent hover:text-foreground",
              multiline ? "top-1.5" : "top-1/2 -translate-y-1/2",
            )}
          >
            <Braces className="size-4" />
          </button>
        </PopoverTrigger>
        <PopoverContent align="end" className="w-64">
          <p className="text-xs font-semibold text-foreground">Insertar variable</p>
          <p className="mt-0.5 text-xs text-muted-foreground">Se inserta como {`{{variable}}`} en el cursor.</p>

          {used.length > 0 && (
            <div className="mt-3">
              <p className="mb-1 text-[11px] uppercase tracking-wide text-muted-foreground">Usadas</p>
              <div className="flex flex-wrap gap-1">
                {used.map((v) => (
                  <button
                    key={v}
                    type="button"
                    onClick={() => insert(v)}
                    className="rounded bg-primary/10 px-1.5 py-0.5 font-mono text-xs text-primary transition-colors hover:bg-primary/20"
                  >{`{{${v}}}`}</button>
                ))}
              </div>
            </div>
          )}

          <div className="mt-3">
            <p className="mb-1 text-[11px] uppercase tracking-wide text-muted-foreground">Nueva variable</p>
            <form
              className="flex items-center gap-1.5"
              onSubmit={(e) => {
                e.preventDefault();
                insert(name);
              }}
            >
              <Input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="firstName"
                className="h-8 font-mono text-xs"
                autoFocus
              />
              <button
                type="submit"
                disabled={!name.trim()}
                className="flex h-8 shrink-0 items-center gap-1 rounded-md bg-primary px-2 text-xs font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
              >
                <Plus className="size-3.5" />
                Insertar
              </button>
            </form>
          </div>
        </PopoverContent>
      </Popover>
    </div>
  );
}
