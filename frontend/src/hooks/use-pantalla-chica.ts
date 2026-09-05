import { useEffect, useState } from "react";

/**
 * El corte entre "cabe al lado" y "no cabe": el `lg` de Tailwind, 1024 px.
 *
 * Mismo número que usa PEDIDO (`front/src/hooks/pantalla.ts`) para la misma decisión:
 * por debajo, los modales se abren como cajón; por encima, se quedan como modal.
 */
export const CORTE_ANCHO = 1024;

/**
 * Si la pantalla es de móvil o tablet.
 *
 * Decisión del dueño del producto (05/09/2026): en pantalla pequeña un modal se come el
 * sitio, deja el contenido apretado y el gesto de cerrar no cae donde la mano espera. El
 * cajón entra desde abajo, ocupa lo que necesita y se cierra arrastrando. En escritorio el
 * modal centrado va mejor.
 *
 * Arranca en `false` a propósito: en el primer pintado no se sabe el ancho todavía, y
 * suponer "pequeña" haría parpadear un cajón en escritorio antes de convertirse en modal.
 */
export function usePantallaChica(): boolean {
  const [chica, setChica] = useState(false);

  useEffect(() => {
    // `matchMedia`, no `resize`: avisa sólo cuando se CRUZA el corte.
    const consulta = window.matchMedia(`(max-width: ${CORTE_ANCHO - 1}px)`);

    setChica(consulta.matches);

    const alCambiar = (e: MediaQueryListEvent) => setChica(e.matches);

    consulta.addEventListener("change", alCambiar);

    return () => consulta.removeEventListener("change", alCambiar);
  }, []);

  return chica;
}
