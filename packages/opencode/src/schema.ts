import { Launch_Kind } from "agx-client/protocol";
import { ConnectionOptions, LifecycleToken, McpGrant } from "agx-client/schema";
import * as Schema from "effect/Schema";

export const Options = Schema.Struct({
  ...ConnectionOptions.fields,
  harness: Schema.optionalKey(Schema.String),
  scopeDirectory: Schema.optionalKey(Schema.String),
  mcpDirectory: Schema.optionalKey(Schema.String),
  codemode: Schema.optionalKey(Schema.Boolean),
});
export type Options = typeof Options.Type;

export const ResourcePath = Schema.String.check(
  Schema.makeFilter(
    (value) =>
      value.length > 0 &&
      !value.includes("\\") &&
      !value.includes(":") &&
      !value.includes("\0") &&
      value.split("/").every((part) => part !== "" && part !== "." && part !== ".."),
  ),
);

export const SkillTree = Schema.Struct({
  id: Schema.NonEmptyString,
  name: Schema.NonEmptyString,
  digest: Schema.String.check(Schema.isPattern(/^[a-f0-9]{64}$/)),
  files: Schema.Array(
    Schema.Struct({
      path: ResourcePath,
      content: Schema.Uint8Array,
      executable: Schema.Boolean,
    }),
  ),
  directories: Schema.Array(ResourcePath),
});
export type SkillTree = typeof SkillTree.Type;

export const Frontmatter = Schema.Struct({
  description: Schema.optionalKey(Schema.String),
  "disable-model-invocation": Schema.optionalKey(Schema.Boolean),
});

export const Snapshot = Schema.Struct({
  id: Schema.NonEmptyString,
  directory: Schema.NonEmptyString,
  harness: Schema.String,
  requiredEnvironment: Schema.Array(Schema.NonEmptyString),
});

export const Manifest = Schema.Struct({
  revision: Schema.NonEmptyString,
  snapshot: Snapshot,
  resources: Schema.Array(
    Schema.Struct({
      resource: Schema.Struct({
        resource: Schema.Union([
          Schema.Struct({ case: Schema.Literal("skillId"), value: Schema.NonEmptyString }),
          Schema.Struct({ case: Schema.Literal("instructionsId"), value: Schema.NonEmptyString }),
          Schema.Struct({ case: Schema.Literal("mcpId"), value: Schema.NonEmptyString }),
        ]),
      }),
      digest: Schema.NonEmptyString,
    }),
  ),
  settings: Schema.Array(
    Schema.Struct({ path: Schema.Array(Schema.String), valueJson: Schema.String }),
  ),
});
export type Manifest = typeof Manifest.Type;

export const ResourceWatchEvent = Schema.Struct({
  state: Schema.Union([
    Schema.Struct({ case: Schema.Literal("manifest"), value: Manifest }),
    Schema.Struct({
      case: Schema.Literal("invalid"),
      value: Schema.Struct({
        diagnostics: Schema.Array(Schema.Struct({ code: Schema.String, summary: Schema.String })),
      }),
    }),
  ]),
});

export const LeaseEnvelope = Schema.Struct({
  launch: Schema.Struct({
    id: Schema.NonEmptyString,
    snapshotId: Schema.NonEmptyString,
    directory: Schema.NonEmptyString,
    harness: Schema.String,
    kind: Schema.Literal(Launch_Kind.RESOURCES),
  }),
  lifecycleToken: LifecycleToken,
});

export const Prepared = Schema.Struct({
  ...LeaseEnvelope.fields,
  content: Schema.Struct({ skills: Schema.Array(SkillTree), instructions: Schema.String }),
  mcp: Schema.Array(Schema.Struct({ id: Schema.NonEmptyString, url: Schema.URLFromString })),
  grant: McpGrant,
  renewAfterSeconds: Schema.Int.check(Schema.isGreaterThan(0)),
  mcpDirectory: Schema.String,
  manifest: Manifest,
});
export type Prepared = typeof Prepared.Type;
