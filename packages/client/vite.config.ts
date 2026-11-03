import { defineConfig } from "vite-plus";
import { copyFile } from "node:fs/promises";

export default defineConfig({
  pack: {
    entry: {
      index: "src/index.ts",
      AgxClient: "src/AgxClient.ts",
      AgxConnection: "src/AgxConnection.ts",
      schema: "src/schema.ts",
      error: "src/error.ts",
      protocol: "../../gen/typescript/agx/v1/agx_pb.ts",
    },
    format: ["esm"],
    dts: true,
    deps: { neverBundle: [/^effect(?:\/|$)/] },
    onSuccess: async () => {
      await copyFile("../../LICENSE", "dist/LICENSE");
    },
  },
});
