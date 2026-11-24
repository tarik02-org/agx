import { copyFile, cp, readFile, writeFile } from "node:fs/promises";

import * as Schema from "effect/Schema";
import license from "unplugin-license/rolldown";
import type { PackUserConfig } from "vite-plus/pack";

const PackageMetadata = Schema.Struct({
  name: Schema.String,
  version: Schema.String,
  description: Schema.String,
  license: Schema.String,
});
const BundleManifest = Schema.fromJsonString(
  Schema.Struct({
    ...PackageMetadata.fields,
    type: Schema.Literal("module"),
    exports: Schema.Literal("./index.mjs"),
  }),
);
const hostPackages = [/^effect(?:\/|$)/, /^@opencode\/plugin(?:\/|$)/];

export const gitBundle: PackUserConfig = {
  name: "git",
  entry: ["src/index.ts"],
  outDir: "dist/git",
  format: ["esm"],
  dts: false,
  plugins: [license({ output: { file: "THIRD_PARTY_LICENSES.md" } })],
  deps: {
    neverBundle: hostPackages,
    alwaysBundle: (id) => !hostPackages.some((pattern) => pattern.test(id)),
    onlyBundle: false,
    onlyImport: hostPackages,
  },
  onSuccess: async () => {
    const metadata = Schema.decodeUnknownSync(Schema.fromJsonString(PackageMetadata))(
      await readFile("package.json", "utf8"),
    );
    await writeFile(
      "dist/git/package.json",
      Schema.encodeSync(BundleManifest)({ ...metadata, type: "module", exports: "./index.mjs" }),
    );
    await copyFile("README.md", "dist/git/README.md");
    await copyFile("../../LICENSE", "dist/git/LICENSE");
    await cp("licenses", "dist/git/licenses", { recursive: true });
  },
};
