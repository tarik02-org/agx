import tailwindcss from "@tailwindcss/vite";
import { tanstackRouter } from "@tanstack/router-plugin/vite";
import react from "@vitejs/plugin-react";
import * as NodeURL from "node:url";
import { defineConfig } from "vite-plus";

// `vp dev` serves the UI on its own port and forwards RPCs and the sign-in to a running daemon,
// whose address agx daemon status prints. The daemon accepts only its own Origin.
const daemon = process.env["AGX_DAEMON_URL"] ?? "http://127.0.0.1:7623";

export default defineConfig({
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
    },
  },
  build: {
    outDir: "../internal/ui/dist",
    emptyOutDir: true,
    assetsDir: "assets",
  },
  fmt: {
    ignorePatterns: [
      "src/gen/**",
      "src/routeTree.gen.ts",
      "pnpm-lock.yaml",
      "src/components/ui/**",
      "src/hooks/use-mobile.ts",
    ],
    sortPackageJson: {},
  },
  lint: {
    ignorePatterns: ["src/gen/**", "src/routeTree.gen.ts"],
    plugins: ["eslint", "oxc", "react", "unicorn", "typescript", "import"],
    categories: {
      correctness: "error",
      suspicious: "error",
      perf: "error",
    },
    rules: {
      "react/react-in-jsx-scope": "off",
      "unicorn/no-array-sort": "off",
      "eslint/no-shadow": "off",
      // Effect discriminates unions and errors by _tag.
      "eslint/no-underscore-dangle": ["error", { allow: ["_tag"] }],
      "import/no-cycle": "error",
      "typescript/no-explicit-any": "error",
      "typescript/no-non-null-assertion": "error",
      "import/no-unassigned-import": ["error", { allow: ["**/*.css", "@fontsource-variable/*"] }],
    },
    overrides: [
      {
        // shadcn generates these; they change through its CLI (add --diff), not by hand.
        files: ["src/components/ui/**", "src/hooks/use-mobile.ts"],
        rules: {
          "react/set-state-in-effect": "off",
          "react/no-object-type-as-default-prop": "off",
          "react/no-array-index-key": "off",
        },
      },
    ],
  },
  test: {
    include: ["src/**/*.test.ts", "src/**/*.test.tsx"],
  },
});
