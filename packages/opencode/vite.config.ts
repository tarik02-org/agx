import { defineConfig } from "vite-plus";
import { copyFile } from "node:fs/promises";

export default defineConfig({
  pack: {
    entry: ["src/index.ts"],
    format: ["esm"],
    dts: true,
    onSuccess: async () => {
      await copyFile("../../LICENSE", "dist/LICENSE");
      await copyFile("node_modules/@effect/platform-node/LICENSE", "dist/LICENSE.effect");
    },
    deps: {
      neverBundle: [/^effect(?:\/|$)/, /^@opencode\//],
      alwaysBundle: ["@effect/platform-node", "@effect/platform-node-shared"],
      onlyBundle: ["@effect/platform-node", "@effect/platform-node-shared"],
    },
  },
});
