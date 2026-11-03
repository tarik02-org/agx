import * as Schema from "effect/Schema";
import { Launch_Kind } from "../../../gen/typescript/agx/v1/agx_pb.ts";

export const Options = Schema.Struct({
  executable: Schema.optionalKey(Schema.String),
  config: Schema.optionalKey(Schema.String),
  discovery: Schema.optionalKey(Schema.String),
  harness: Schema.optionalKey(Schema.String),
  scopeDirectory: Schema.optionalKey(Schema.String),
  mcpDirectory: Schema.optionalKey(Schema.String),
  codemode: Schema.optionalKey(Schema.Boolean),
});
export type Options = typeof Options.Type;

export const LifecycleToken = Schema.NonEmptyString.pipe(Schema.brand("agx/LifecycleToken"));
export type LifecycleToken = typeof LifecycleToken.Type;
export const McpGrant = Schema.NonEmptyString.pipe(Schema.brand("agx/McpGrant"));
export const ManagementToken = Schema.NonEmptyString.pipe(Schema.brand("agx/ManagementToken"));

export const Bootstrap = Schema.Struct({
  version: Schema.Literal(1),
  data: Schema.Struct({ discovery: Schema.String }),
});

export const Discovery = Schema.Struct({
  url: Schema.URLFromString.check(
    Schema.makeFilter(
      (url) =>
        url.protocol === "http:" &&
        url.hostname === "127.0.0.1" &&
        url.port !== "" &&
        url.username === "" &&
        url.password === "" &&
        url.pathname === "/" &&
        url.search === "" &&
        url.hash === "",
    ),
  ),
  token: ManagementToken,
  instance_id: Schema.NonEmptyString,
  config: Schema.NonEmptyString,
});
export type Discovery = typeof Discovery.Type;

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
});
export type Prepared = typeof Prepared.Type;
