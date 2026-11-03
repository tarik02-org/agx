import * as Schema from "effect/Schema";
import * as PlatformError from "effect/PlatformError";
import type { ClientFailure } from "agx-client/error";

export class IntegrationError extends Schema.TaggedError<IntegrationError>()("IntegrationError", {
  operation: Schema.String,
  reason: Schema.Literals([
    "options",
    "bootstrap",
    "discovery",
    "content",
    "filesystem",
    "settings",
  ]),
  cause: Schema.optionalKey(
    Schema.Union([
      Schema.instanceOf(PlatformError.PlatformError),
      Schema.instanceOf(Schema.SchemaError),
      Schema.Struct({ kind: Schema.Literal("yaml"), code: Schema.String }),
    ]),
  ),
}) {
  override get message() {
    return `AGX ${this.operation} failed (${this.reason})`;
  }
}

export type IntegrationFailure = IntegrationError | ClientFailure;
