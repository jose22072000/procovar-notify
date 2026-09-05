import * as React from "react";
import * as DialogPrimitive from "@radix-ui/react-dialog";
import { X } from "lucide-react";
import { cn } from "@/lib/utils";
import { usePantallaChica } from "@/hooks/use-pantalla-chica";

const Dialog = DialogPrimitive.Root;
const DialogTrigger = DialogPrimitive.Trigger;
const DialogClose = DialogPrimitive.Close;

const DialogOverlay = React.forwardRef<
  React.ElementRef<typeof DialogPrimitive.Overlay>,
  React.ComponentPropsWithoutRef<typeof DialogPrimitive.Overlay>
>(({ className, ...props }, ref) => (
  <DialogPrimitive.Overlay
    ref={ref}
    className={cn(
      "fixed inset-0 z-50 bg-black/40 data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0",
      className,
    )}
    {...props}
  />
));
DialogOverlay.displayName = DialogPrimitive.Overlay.displayName;

/**
 * El mismo contenido, sólo cambia el envase.
 *
 * Por debajo de 1024px (`usePantallaChica`) esto se dibuja como un cajón que sube desde
 * abajo: en el móvil un modal centrado se come el sitio y el gesto de cerrar no cae donde
 * la mano espera. En escritorio se queda el modal centrado de siempre.
 *
 * No hay una librería de Drawer instalada aquí (Radix trae Dialog, no Drawer), así que el
 * cajón es el MISMO `DialogPrimitive.Content` con otras clases de Tailwind — nada de
 * duplicar el contenido de cada pantalla, sólo estas clases cambian.
 *
 * La ✕ es la misma de siempre y sigue dibujándose dentro de este contenedor, que nunca se
 * hace transparente: si el día de mañana alguien pone `bg-transparent` aquí, la ✕
 * desaparece con el fondo y hay que escribirla a mano en la cabecera de cada pantalla.
 */
const DialogContent = React.forwardRef<
  React.ElementRef<typeof DialogPrimitive.Content>,
  React.ComponentPropsWithoutRef<typeof DialogPrimitive.Content>
>(({ className, children, ...props }, ref) => {
  const chica = usePantallaChica();

  return (
    <DialogPrimitive.Portal>
      <DialogOverlay />
      <DialogPrimitive.Content
        ref={ref}
        className={cn(
          "fixed z-50 grid gap-4 overflow-y-auto border bg-background p-6 shadow-lg",
          "data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0",
          chica
            ? // Cajón: entra desde abajo, ancho completo, sólo el borde de arriba redondeado.
              // El `className` de cada pantalla (anchos tipo `sm:max-w-xl`) es cosa del
              // modal de escritorio y aquí se ignora a propósito: un cajón de 700px de
              // ancho no tiene por qué quedarse más angosto que la pantalla.
              "inset-x-0 bottom-0 top-auto w-full max-h-[85dvh] rounded-t-xl rounded-b-none border-border data-[state=closed]:slide-out-to-bottom data-[state=open]:slide-in-from-bottom"
            : cn(
                "left-1/2 top-1/2 w-[calc(100%-2rem)] max-w-lg max-h-[calc(100dvh-2rem)] -translate-x-1/2 -translate-y-1/2 rounded-xl border-border data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95",
                className,
              ),
        )}
        {...props}
      >
        {children}
        <DialogPrimitive.Close className="absolute right-4 top-4 rounded-md opacity-70 outline-none transition-opacity hover:opacity-100 focus-visible:ring-2 focus-visible:ring-ring/40">
          <X className="size-4" />
          <span className="sr-only">Cerrar</span>
        </DialogPrimitive.Close>
      </DialogPrimitive.Content>
    </DialogPrimitive.Portal>
  );
});
DialogContent.displayName = DialogPrimitive.Content.displayName;

function DialogHeader({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("flex flex-col gap-1.5 text-left", className)} {...props} />;
}

function DialogFooter({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("flex flex-col-reverse gap-2 sm:flex-row sm:justify-end", className)} {...props} />;
}

const DialogTitle = React.forwardRef<
  React.ElementRef<typeof DialogPrimitive.Title>,
  React.ComponentPropsWithoutRef<typeof DialogPrimitive.Title>
>(({ className, ...props }, ref) => (
  <DialogPrimitive.Title ref={ref} className={cn("text-lg font-semibold tracking-tight", className)} {...props} />
));
DialogTitle.displayName = DialogPrimitive.Title.displayName;

const DialogDescription = React.forwardRef<
  React.ElementRef<typeof DialogPrimitive.Description>,
  React.ComponentPropsWithoutRef<typeof DialogPrimitive.Description>
>(({ className, ...props }, ref) => (
  <DialogPrimitive.Description ref={ref} className={cn("text-sm text-muted-foreground", className)} {...props} />
));
DialogDescription.displayName = DialogPrimitive.Description.displayName;

export {
  Dialog,
  DialogTrigger,
  DialogClose,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
};
