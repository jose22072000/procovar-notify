import { Component, type ErrorInfo, type ReactNode } from "react";

interface Props {
  children: ReactNode;
  // Fallback a medida; recibe una función para reintentar el render.
  fallback?: (reset: () => void) => ReactNode;
}
interface State {
  error: Error | null;
}

// ErrorBoundary captura excepciones de render de su subárbol y muestra un
// fallback en vez de dejar la SPA en blanco. Aísla el fallo: cuanto más cerca
// del componente se coloca, más pequeño es el trozo de UI que se pierde (el
// resto del panel —cabecera, navegación, otras pestañas— sigue vivo).
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // No hay telemetría en el cliente: la consola es el único rastro de
    // diagnóstico. No se registran datos sensibles, solo el error de render.
    console.error("ErrorBoundary:", error, info.componentStack);
  }

  reset = () => this.setState({ error: null });

  render() {
    if (this.state.error) {
      if (this.props.fallback) return this.props.fallback(this.reset);
      return (
        <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed border-destructive/40 bg-destructive/5 py-10 text-center">
          <p className="text-sm font-medium text-destructive">Algo ha fallado al mostrar esta sección.</p>
          <p className="max-w-md text-xs text-muted-foreground">
            El error se ha contenido aquí; el resto del panel sigue funcionando. Puedes reintentar o recargar la página.
          </p>
          <button
            type="button"
            onClick={this.reset}
            className="rounded-md border border-input bg-card px-3 py-1.5 text-sm font-medium transition-colors hover:bg-accent"
          >
            Reintentar
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}
