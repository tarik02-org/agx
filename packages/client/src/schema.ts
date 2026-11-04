import * as Schema from "effect/Schema";

export const ConnectionOptions = Schema.Struct({
  executable: Schema.optionalKey(Schema.NonEmptyString),
  config: Schema.optionalKey(Schema.String),
  discovery: Schema.optionalKey(Schema.String),
});
export type ConnectionOptions = typeof ConnectionOptions.Type;

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
