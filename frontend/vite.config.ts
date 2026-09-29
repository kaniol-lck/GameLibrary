import { fileURLToPath } from "node:url";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      // Import application code as "@/..." rather than by relative depth.
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  build: {
    // Wails embeds frontend/dist into the executable.
    outDir: "dist",
    sourcemap: false,
  },
});
