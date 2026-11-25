import * as Option from "effect/Option";
import * as Schema from "effect/Schema";

/** The part of JSON Schema that tool input forms read; everything else stays opaque. */
export interface JsonSchemaNode {
  readonly type?: string | ReadonlyArray<string> | undefined;
  readonly title?: string | undefined;
  readonly description?: string | undefined;
  readonly enum?: ReadonlyArray<unknown> | undefined;
  readonly default?: unknown;
  readonly properties?: { readonly [key: string]: JsonSchemaNode } | undefined;
  readonly required?: ReadonlyArray<string> | undefined;
  readonly items?: JsonSchemaNode | undefined;
}

export const JsonSchemaNode: Schema.Codec<JsonSchemaNode> = Schema.Struct({
  type: Schema.optional(Schema.Union([Schema.String, Schema.Array(Schema.String)])),
  title: Schema.optional(Schema.String),
  description: Schema.optional(Schema.String),
  enum: Schema.optional(Schema.Array(Schema.Unknown)),
  default: Schema.optional(Schema.Unknown),
  properties: Schema.optional(
    Schema.Record(
      Schema.String,
      Schema.suspend(() => JsonSchemaNode),
    ),
  ),
  required: Schema.optional(Schema.Array(Schema.String)),
  items: Schema.optional(Schema.suspend(() => JsonSchemaNode)),
});

const decodeSchema = Schema.decodeUnknownOption(Schema.fromJsonString(JsonSchemaNode));

/** A tool's input schema as the daemon sends it, or an empty object schema when it has none. */
export const parseSchema = (json: string): JsonSchemaNode =>
  json === ""
    ? { type: "object" }
    : Option.getOrElse(decodeSchema(json), () => ({ type: "object" }));

/** The schema's single type, ignoring "null" in a union such as ["string", "null"]. */
export const schemaType = (schema: JsonSchemaNode): string | undefined => {
  if (typeof schema.type === "string") {
    return schema.type;
  }
  return schema.type?.find((type) => type !== "null");
};
