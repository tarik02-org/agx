import { defineConfig } from "vite-plus";

export default defineConfig({
  lint: {
    ignorePatterns: ["**/gen/**", "**/dist/**"],
    plugins: ["typescript", "unicorn", "oxc", "import", "promise"],
    categories: { correctness: "error", suspicious: "error", perf: "error" },
    overrides: [
      {
        files: ["packages/opencode/test/**"],
        // Process readiness checks must poll sequentially.
        rules: { "no-await-in-loop": "off" },
      },
    ],
    options: {
      typeAware: true,
      typeCheck: true,
      denyWarnings: true,
      reportUnusedDisableDirectives: "error",
    },
    rules: {
      "no-underscore-dangle": ["error", { allow: ["_tag"] }],
      eqeqeq: ["error", "always"],
      curly: ["error", "all"],
      "typescript/no-explicit-any": "error",
      "typescript/no-non-null-assertion": "error",
      "typescript/consistent-type-imports": "error",
      "typescript/no-misused-promises": "error",
      "typescript/no-unsafe-argument": "error",
      "typescript/no-unsafe-assignment": "error",
      "typescript/no-unsafe-call": "error",
      "typescript/no-unsafe-member-access": "error",
      "typescript/no-unsafe-return": "error",
      "typescript/only-throw-error": "error",
      "typescript/switch-exhaustiveness-check": "error",
      "no-restricted-properties": [
        "error",
        ...[
          "mapError",
          "mapErrorEager",
          "mapBoth",
          "mapBothEager",
          "catch",
          "catchEager",
          "catchCause",
          "catchDefect",
          "orDie",
          "orElseSucceed",
          "ignore",
          "ignoreCause",
          "option",
        ].map((property) => ({
          object: "Effect",
          property,
          message:
            "Handle specific error tags or reasons explicitly with catchTag, catchTags, or catchReason.",
        })),
      ],
    },
  },
  fmt: {
    ignorePatterns: ["**/gen/**", "**/dist/**"],
    sortImports: true,
    sortPackageJson: true,
  },
});
