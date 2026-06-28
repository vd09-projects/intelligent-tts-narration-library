/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Earshot Vite config.
//
// DEV PROXY (dev only): `make earshot-dev` forwards the server-owned routes to
// the local narrate-server (#109). A built bundle (`make earshot-build`) carries
// NO proxy — it is a compile/smoke check only, never a runnable deployment.
// See README "Build is compile-smoke only".
//
// The proxy target is overridable with VITE_NARRATE_SERVER (defaults to the
// narrate-server Makefile default 127.0.0.1:8080).
const SERVER = process.env.VITE_NARRATE_SERVER ?? "http://127.0.0.1:8080";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      // The three server-owned route prefixes. `audio_url` is opaque (D4) and
      // resolves under /audio, so proxying /audio covers it without the client
      // ever parsing a render_id.
      "/sessions": { target: SERVER, changeOrigin: true },
      "/narrate": { target: SERVER, changeOrigin: true },
      "/audio": { target: SERVER, changeOrigin: true },
    },
  },
  test: {
    globals: true,
    environment: "jsdom",
    setupFiles: ["./vitest.setup.ts"],
    css: false,
  },
});
