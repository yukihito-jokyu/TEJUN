import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { configDefaults, defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: { outDir: "../cmd/tejun/frontend/dist", emptyOutDir: true },
  resolve: { alias: { "@": new URL("./src", import.meta.url).pathname } },
  test: { environment: "jsdom", exclude: [...configDefaults.exclude, "e2e/**"] },
});
