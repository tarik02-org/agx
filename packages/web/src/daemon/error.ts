import { Code, ConnectError } from "@connectrpc/connect";
import { ErrorDetailSchema } from "agx-client/protocol";
import * as Schema from "effect/Schema";

// What went wrong, from the Connect code the daemon answered with: `config_invalid` when it
// carries a configuration error detail, `needs_login` and `mcp_failed` for an MCP server's
// failures, `unreachable` when no daemon answers, and `canceled` when the page itself gave up
// on a request. Pages branch on these, never on Connect codes or messages.
export const DaemonFailure = Schema.Literals([
  "invalid_request",
  "not_found",
  "conflict",
  "failed_precondition",
  "needs_login",
  "config_invalid",
  "unavailable",
  "unauthenticated",
  "timeout",
  "internal",
  "mcp_failed",
  "unreachable",
  "canceled",
]);
export type DaemonFailure = typeof DaemonFailure.Type;

export class DaemonError extends Schema.TaggedError<DaemonError>()("DaemonError", {
  operation: Schema.String,
  reason: DaemonFailure,
  message: Schema.String,
}) {}

const fromCode = (code: Code): DaemonFailure => {
  switch (code) {
    case Code.Unauthenticated:
    case Code.PermissionDenied:
      return "unauthenticated";
    case Code.InvalidArgument:
    case Code.OutOfRange:
      return "invalid_request";
    case Code.NotFound:
      return "not_found";
    case Code.AlreadyExists:
    case Code.Aborted:
      return "conflict";
    case Code.FailedPrecondition:
      return "failed_precondition";
    case Code.DeadlineExceeded:
      return "timeout";
    case Code.Unavailable:
      return "unreachable";
    case Code.Canceled:
      return "canceled";
    case Code.ResourceExhausted:
      return "unavailable";
    case Code.Unknown:
    case Code.Unimplemented:
    case Code.Internal:
    case Code.DataLoss:
      return "internal";
  }
};

// The operations that talk to an MCP server, whose failures the daemon reports in the codes
// the server's state maps to.
const mcpOperations = new Set(["listTools", "callTool"]);

/** Classifies a failed daemon call, which the Connect client reports as a thrown value. */
export const daemonError = (operation: string, cause: unknown): DaemonError => {
  const error = ConnectError.from(cause);
  // A request that never reached the daemon fails in fetch, which throws a TypeError; the
  // Connect client wraps it as Unknown, the code a daemon's plain error also has.
  if (error.cause instanceof TypeError) {
    return new DaemonError({ operation, reason: "unreachable", message: error.rawMessage });
  }
  let reason = fromCode(error.code);
  // Only configuration that does not load comes with a detail.
  if (error.findDetails(ErrorDetailSchema).length > 0) {
    reason = "config_invalid";
  } else if (mcpOperations.has(operation)) {
    // A server needing a login answers Unauthenticated too; the page's own credential failing
    // is the gate's refusal, which names the management credential.
    if (reason === "unauthenticated" && !error.rawMessage.startsWith("management credential")) {
      reason = "needs_login";
    } else if (reason === "failed_precondition") {
      reason = "mcp_failed";
    }
  }
  return new DaemonError({ operation, reason, message: error.rawMessage });
};
