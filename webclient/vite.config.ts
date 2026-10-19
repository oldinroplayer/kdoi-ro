import { defineConfig } from "vite";

// In development, map/sprite files are served from public/data and public/bgm,
// symlinks to ../ro-client. Builds leave them out (they're ~400 MB): the release
// script (../build.sh) copies them next to the client once, and the game
// server serves them.
export default defineConfig({
  server: { port: 5173 },
  // three.js makes one ~700 kB bundle; that's fine for a game, so no warning.
  build: { copyPublicDir: false, chunkSizeWarningLimit: 2000 },
});
