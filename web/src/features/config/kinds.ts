/** The configuration's sections of definitions, in the order pages list them. */
export const kinds: ReadonlyArray<{
  readonly kind: string;
  readonly label: string;
  /** One definition of the kind, as in "New skill". */
  readonly one: string;
}> = [
  { kind: "skills", label: "Skills", one: "Skill" },
  { kind: "mcp", label: "MCP servers", one: "MCP server" },
  { kind: "instructions", label: "Instructions", one: "Instructions" },
  { kind: "secrets", label: "Secrets", one: "Secret" },
  { kind: "bundles", label: "Bundles", one: "Bundle" },
  { kind: "packages", label: "Packages", one: "Package" },
  { kind: "harnesses", label: "Harnesses", one: "Harness" },
];

/** Kinds scope rules select, so they can be enabled and disabled. */
export const selectable: ReadonlySet<string> = new Set([
  "skills",
  "instructions",
  "mcp",
  "bundles",
]);
