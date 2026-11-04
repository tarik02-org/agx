import { copyFile } from "node:fs/promises";

import { defineConfig } from "vite-plus";

export default defineConfig({
  run: {
    tasks: {
      build: {
        command: "vp pack",
        dependsOn: [{ task: "build", from: "dependencies" }],
      },
      test: {
        command: "node --test test/*.test.ts",
        dependsOn: ["build"],
        cache: false,
      },
    },
  },
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
