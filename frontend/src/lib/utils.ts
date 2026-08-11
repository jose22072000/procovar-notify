import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

// cn combina clases condicionales (clsx) y resuelve conflictos de Tailwind (twMerge).
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
