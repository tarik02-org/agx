import { copyFile } from "node:fs/promises";

import { defineConfig } from "vite-plus";

import { gitBundle } from "./vite.bundle.config.ts";

export default defineConfig({
  run: {
    tasks: {
      build: {
        command: "vp pack --filter module",
        dependsOn: [{ task: "build", from: "dependencies" }],
      },
      bundle: {
        command: "vp pack --filter git",
        dependsOn: [{ task: "build", from: "dependencies" }],
      },
      test: {
        command: "node --test test/*.test.ts",
        dependsOn: ["bundle"],
        cache: false,
      },
    },
  },
  pack: [
    {
      name: "module",
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
    gitBundle,
  ],
});
