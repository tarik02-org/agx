import tailwindcss from "@tailwindcss/vite";
import { tanstackRouter } from "@tanstack/router-plugin/vite";
import react from "@vitejs/plugin-react";
import * as NodeURL from "node:url";
import { defineConfig, type Plugin } from "vite-plus";

// `vp dev` serves the UI on its own port and forwards RPCs and the sign-in to a running daemon,
// whose address agx daemon status prints. The daemon accepts only its own Origin.
const daemon = process.env["AGX_DAEMON_URL"] ?? "http://127.0.0.1:7623";

const source = (path: string) => NodeURL.fileURLToPath(new URL(path, import.meta.url));

// The daemon binary embeds every chunk the build emits, lazy or not, so dependencies' code paths
// the UI never takes are swapped out before they are bundled. @pierre/diffs loads these only for
// highlighters other than its default "shiki-js" (its own engine, Shiki's WASM one) and for themes
// not registered with it.
const unusedModules = new Set([
  "shiki/wasm",
  "shiki/engine/oniguruma",
  "@pierre/highlights/themes/loader",
  "@pierre/theming/themes",
]);

const trimDependencies: Plugin = {
  name: "agx:trim-dependencies",
  enforce: "pre",
  resolveId(source) {
    return unusedModules.has(source) ? `\0unused:${source}` : null;
  },
  load(id) {
    if (
      id.startsWith("\0unused:") ||
      id.endsWith("/@pierre/diffs/dist/highlighter/backends/highlights.js")
    ) {
      return 'throw new Error("not part of the agx UI build");';
    }
    // codemirror-json-schema renders hover documentation with markdown-it plus a Shiki 1
    // highlighter it builds at import time, WASM engine included. Schema descriptions carry no code.
    if (id.endsWith("/codemirror-json-schema/dist/utils/markdown.js")) {
      return [
        'import md from "markdown-it";',
        "const renderer = md({ linkify: true, typographer: true });",
        "export const renderMarkdown = (markdown, inline = true) =>",
        "  inline ? renderer.renderInline(markdown) : renderer.render(markdown);",
      ].join("\n");
    }
    return null;
  },
};

export default defineConfig({
  base: "/ui/",
  resolve: {
    alias: [
      { find: "~", replacement: source("./src") },
      { find: /^shiki\/langs$/, replacement: source("./src/lib/highlighter/languages.ts") },
    ],
  },
  plugins: [
    tanstackRouter({ target: "react", autoCodeSplitting: true }),
    react({ compiler: true }),
    trimDependencies,
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
    outDir: "../internal/ui/dist",
    emptyOutDir: true,
    assetsDir: "assets",
    // The daemon serves these from localhost. The largest chunk, CodeMirror with its JSON Schema
    // support, loads only once a YAML or Markdown editor shows.
    chunkSizeWarningLimit: 900,
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
          "react/jsx-no-constructed-context-values": "off",
        },
      },
    ],
  },
  test: {
    include: ["src/**/*.test.ts", "src/**/*.test.tsx"],
  },
});
