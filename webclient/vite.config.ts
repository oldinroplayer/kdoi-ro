import { defineConfig } from "vite";

// Map/sprite files are served from public/data, a symlink to ../ro-client/data.
export default defineConfig({
  server: { port: 5173 },
});
