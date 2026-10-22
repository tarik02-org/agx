/** The configuration's sections of definitions, in the order pages list them. */
export const kinds: ReadonlyArray<{ readonly kind: string; readonly label: string }> = [
  { kind: "skills", label: "Skills" },
  { kind: "mcp", label: "MCP servers" },
  { kind: "instructions", label: "Instructions" },
  { kind: "secrets", label: "Secrets" },
  { kind: "bundles", label: "Bundles" },
  { kind: "packages", label: "Packages" },
  { kind: "harnesses", label: "Harnesses" },
];

/** Kinds scope rules select, so they can be enabled and disabled. */
export const selectable: ReadonlySet<string> = new Set([
  "skills",
  "instructions",
  "mcp",
  "bundles",
]);
