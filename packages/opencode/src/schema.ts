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

export const AgentContent = Schema.Struct({
  id: Schema.NonEmptyString,
  name: Schema.NonEmptyString,
  description: Schema.String,
  prompt: Schema.String,
  native: Schema.optional(Schema.Record(Schema.String, Schema.Unknown)),
});

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
  content: Schema.Struct({
    skills: Schema.Array(SkillTree),
    instructions: Schema.String,
    agents: Schema.Array(AgentContent),
  }),
  mcp: Schema.Array(Schema.Struct({ id: Schema.NonEmptyString, url: Schema.URLFromString })),
  grant: McpGrant,
  renewAfterSeconds: Schema.Int.check(Schema.isGreaterThan(0)),
  mcpDirectory: Schema.String,
});
export type Prepared = typeof Prepared.Type;
