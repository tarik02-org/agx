import * as Option from "effect/Option";
import * as Schema from "effect/Schema";

// MCP tool result content, as CallToolResponse.content_json carries it.
const TextContent = Schema.Struct({ type: Schema.Literal("text"), text: Schema.String });
const ImageContent = Schema.Struct({
  type: Schema.Literal("image"),
  data: Schema.String,
  mimeType: Schema.String,
});
const AudioContent = Schema.Struct({
  type: Schema.Literal("audio"),
  data: Schema.String,
  mimeType: Schema.String,
});
const ResourceLink = Schema.Struct({
  type: Schema.Literal("resource_link"),
  uri: Schema.String,
  name: Schema.optional(Schema.String),
  description: Schema.optional(Schema.String),
  mimeType: Schema.optional(Schema.String),
});
const EmbeddedResource = Schema.Struct({
  type: Schema.Literal("resource"),
  resource: Schema.Struct({
    uri: Schema.String,
    mimeType: Schema.optional(Schema.String),
    text: Schema.optional(Schema.String),
    blob: Schema.optional(Schema.String),
  }),
});

export const ContentItem = Schema.Union([
  TextContent,
  ImageContent,
  AudioContent,
  ResourceLink,
  EmbeddedResource,
]);
export type ContentItem = typeof ContentItem.Type;

/** One content item, or the raw value when it is of a kind the inspector does not render. */
export type DecodedContent =
  | { readonly _tag: "Known"; readonly item: ContentItem }
  | { readonly _tag: "Unknown"; readonly value: unknown };

const decodeList = Schema.decodeUnknownOption(Schema.fromJsonString(Schema.Array(Schema.Unknown)));
const decodeItem = Schema.decodeUnknownOption(ContentItem);

export const decodeContent = (json: string): ReadonlyArray<DecodedContent> =>
  json === ""
    ? []
    : Option.getOrElse(decodeList(json), () => []).map((value) =>
        Option.match(decodeItem(value), {
          onNone: (): DecodedContent => ({ _tag: "Unknown", value }),
          onSome: (item): DecodedContent => ({ _tag: "Known", item }),
        }),
      );
