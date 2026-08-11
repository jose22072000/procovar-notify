/// <reference types="vitest/config" />
import path from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// En desarrollo el proxy reenvía las llamadas a la API Go (evita CORS).
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { "@": path.resolve(__dirname, "./src") },
  },
  server: {
    proxy: {
      "/admin": "http://localhost:8080",
      "/v1": "http://localhost:8080",
      "/healthz": "http://localhost:8080",
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
    css: false,
    coverage: {
      provider: "v8",
      reporter: ["text-summary", "text"],
      // Suelo anti-regresión (ratchet): la cobertura no puede bajar de aquí. El
      // frontend acaba de empezar a cubrirse; subir estos números conforme se
      // añaden tests (Fase 3/siguientes del PLAN-TESTS).
      thresholds: { statements: 11, branches: 70, functions: 45, lines: 11 },
    },
  },
});
