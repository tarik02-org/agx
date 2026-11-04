import { Code, ConnectError } from "@connectrpc/connect";
import * as PlatformError from "effect/PlatformError";
import * as Schema from "effect/Schema";

export class ConnectionError extends Schema.TaggedError<ConnectionError>()("ConnectionError", {
  operation: Schema.String,
  reason: Schema.Literals(["options", "bootstrap", "discovery"]),
  cause: Schema.optionalKey(
    Schema.Union([
      Schema.instanceOf(PlatformError.PlatformError),
      Schema.instanceOf(Schema.SchemaError),
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
  code: Schema.Enum(Code),
  ended: EndState,
  cause: Schema.instanceOf(ConnectError),
}) {
  override get message() {
    return `AGX ${this.operation} failed (RPC code ${this.code}${this.ended === null ? "" : `, ${this.ended}`})`;
  }
}

export type ClientFailure = ConnectionError | DaemonError;
