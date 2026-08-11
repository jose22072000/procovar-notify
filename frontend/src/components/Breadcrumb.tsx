import { Link } from "react-router-dom";
import { ChevronRight } from "lucide-react";

export interface Crumb {
  label: string;
  to?: string;
}

// Migas de pan para navegar hacia atrás. El último elemento es la página actual.
export function Breadcrumb({ items }: { items: Crumb[] }) {
  return (
    <nav className="mb-4 flex items-center gap-1.5 text-sm" aria-label="Ruta de navegación">
      {items.map((it, i) => (
        <span key={i} className="flex items-center gap-1.5">
          {i > 0 && <ChevronRight className="size-3.5 text-muted-foreground/50" />}
          {it.to ? (
            <Link to={it.to} className="text-muted-foreground transition-colors hover:text-foreground">
              {it.label}
            </Link>
          ) : (
            <span className="font-medium text-foreground">{it.label}</span>
          )}
        </span>
      ))}
    </nav>
  );
}
