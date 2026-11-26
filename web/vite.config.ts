import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// In dev, run `finnick ui --port 7777 --no-open` and open
// http://localhost:5173/auth?token=<token printed by the CLI>.
const api = { target: "http://127.0.0.1:7777", changeOrigin: true };

export default defineConfig({
  plugins: [react()],
  build: { outDir: "dist", emptyOutDir: true, sourcemap: false },
  server: { proxy: { "/api": api, "/auth": api } },
});
