import type { JSONSchema7, JSONSchema7Definition } from "json-schema";
import * as Predicate from "effect/Predicate";
import * as Schema from "effect/Schema";

/** The configuration JSON Schema, opaque except for what the editors look up in it. */
export type ConfigSchema = JSONSchema7;

// The editors hand the schema to CodeMirror whole, so decoding checks its shape and keeps it all.
export const ConfigSchema = Schema.declare(
  (input: unknown): input is JSONSchema7 =>
    Predicate.hasProperty(input, "$defs") && Predicate.isObject(input.$defs),
);

/** The schema definition each configuration section's entries follow. */
export const definitionNames: { readonly [kind: string]: string } = {
  skills: "Skill",
  instructions: "Instructions",
  mcp: "MCPServer",
  secrets: "Secret",
  bundles: "BundleSpec",
  packages: "Package",
  harnesses: "Harness",
};

/** A schema validating one entry of a configuration section, resolving refs into the whole. */
export const definitionSchema = (schema: ConfigSchema, kind: string): JSONSchema7 => ({
  $ref: `#/$defs/${definitionNames[kind] ?? ""}`,
  $defs: schema.$defs ?? {},
});

const asSchema = (definition: JSONSchema7Definition | undefined): JSONSchema7 | undefined =>
  typeof definition === "object" ? definition : undefined;

/** What the schema says about a field of a section's entries. */
export const fieldDescription = (
  schema: ConfigSchema | undefined,
  kind: string,
  field: string,
): string | undefined =>
  asSchema(asSchema(schema?.$defs?.[definitionNames[kind] ?? ""])?.properties?.[field])
    ?.description;
