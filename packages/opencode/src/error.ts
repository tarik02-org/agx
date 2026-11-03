import * as Schema from "effect/Schema";
import * as PlatformError from "effect/PlatformError";
import { ConnectError } from "@connectrpc/connect";

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

export const EndState = Schema.NullOr(Schema.Literals(["expired", "revoked", "finished"]));

export class DaemonError extends Schema.TaggedError<DaemonError>()("DaemonError", {
  operation: Schema.String,
  code: Schema.Number,
  ended: EndState,
  cause: Schema.instanceOf(ConnectError),
}) {
  override get message() {
    return `AGX ${this.operation} failed (RPC code ${this.code}${this.ended === null ? "" : `, ${this.ended}`})`;
  }
}

export type IntegrationFailure = IntegrationError | DaemonError;
