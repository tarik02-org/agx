import * as NodeURL from "node:url";

import tailwindcss from "@tailwindcss/vite";
import { tanstackRouter } from "@tanstack/router-plugin/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite-plus";

// `vp dev` serves the UI on its own port and forwards RPCs and the sign-in to a running daemon,
// whose address agx daemon status prints. The daemon accepts only its own Origin.
const daemon = process.env["AGX_DAEMON_URL"] ?? "http://127.0.0.1:7623";

export default defineConfig({
  run: {
    tasks: {
      dev: {
        command: "vp dev",
        dependsOn: [{ task: "build", from: "dependencies" }],
        cache: false,
      },
      build: {
        command: "vp build && touch ../../internal/ui/dist/.keep",
        dependsOn: [{ task: "build", from: "dependencies" }],
      },
      test: {
        command: "vp test run",
        dependsOn: [{ task: "build", from: "dependencies" }],
      },
    },
  },
  base: "/ui/",
  resolve: {
    alias: { "~": NodeURL.fileURLToPath(new URL("./src", import.meta.url)) },
  },
  plugins: [
    tanstackRouter({ target: "react", autoCodeSplitting: true }),
    react({ compiler: true }),
    tailwindcss(),
  ],
  server: {
    proxy: {
      "/agx.v1.": { target: daemon, changeOrigin: true, headers: { origin: daemon } },
      "/ui/login": { target: daemon, changeOrigin: true },
      "/ui/schema.json": { target: daemon, changeOrigin: true },
    },
  },
  build: {
    outDir: "../../internal/ui/dist",
    emptyOutDir: true,
    assetsDir: "assets",
    // The daemon serves these from localhost. The largest chunks load only when used: Shiki's
    // grammars and WASM engine, and CodeMirror, whose JSON Schema support brings its own Shiki.
    chunkSizeWarningLimit: 1100,
  },
  test: {
    include: ["src/**/*.test.ts", "src/**/*.test.tsx"],
  },
});
