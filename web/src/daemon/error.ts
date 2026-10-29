import { Code, ConnectError } from "@connectrpc/connect";
import * as Schema from "effect/Schema";

import { ErrorDetail_Reason, ErrorDetailSchema } from "~/gen/agx/v1/agx_pb.ts";

// The daemon's closed set of failure reasons (agx.v1.ErrorDetail), `unreachable` when no daemon
// answers, and `canceled` when the page itself gave up on a request. Pages branch on these, never on Connect codes or messages.
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
  "network",
  "choose_file",
  "unreachable",
  "canceled",
]);
export type DaemonFailure = typeof DaemonFailure.Type;

export class DaemonError extends Schema.TaggedError<DaemonError>()("DaemonError", {
  operation: Schema.String,
  reason: DaemonFailure,
  message: Schema.String,
  /** With choose_file, the fragments the changes could go to. */
  files: Schema.Array(Schema.String),
}) {}

const fromReason = (reason: ErrorDetail_Reason): DaemonFailure | undefined => {
  switch (reason) {
    case ErrorDetail_Reason.UNSPECIFIED:
      return undefined;
    case ErrorDetail_Reason.INVALID_REQUEST:
      return "invalid_request";
    case ErrorDetail_Reason.NOT_FOUND:
      return "not_found";
    case ErrorDetail_Reason.CONFLICT:
      return "conflict";
    case ErrorDetail_Reason.FAILED_PRECONDITION:
      return "failed_precondition";
    case ErrorDetail_Reason.NEEDS_LOGIN:
      return "needs_login";
    case ErrorDetail_Reason.CONFIG_INVALID:
      return "config_invalid";
    case ErrorDetail_Reason.UNAVAILABLE:
      return "unavailable";
    case ErrorDetail_Reason.UNAUTHENTICATED:
      return "unauthenticated";
    case ErrorDetail_Reason.TIMEOUT:
      return "timeout";
    case ErrorDetail_Reason.INTERNAL:
      return "internal";
    case ErrorDetail_Reason.MCP_FAILED:
      return "mcp_failed";
    case ErrorDetail_Reason.NETWORK:
      return "network";
    case ErrorDetail_Reason.CHOOSE_FILE:
      return "choose_file";
  }
};

// fromCode classifies errors the daemon sent without a detail.
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

/** Classifies a failed daemon call, which the Connect client reports as a thrown value. */
export const daemonError = (operation: string, cause: unknown): DaemonError => {
  const error = ConnectError.from(cause);
  // A request that never reached the daemon fails in fetch, which throws a TypeError; the
  // Connect client wraps it as Unknown, the code a daemon's plain error also has.
  if (error.cause instanceof TypeError) {
    return new DaemonError({
      operation,
      reason: "unreachable",
      message: error.rawMessage,
      files: [],
    });
  }
  const detail = error.findDetails(ErrorDetailSchema)[0];
  const reason =
    (detail === undefined ? undefined : fromReason(detail.reason)) ?? fromCode(error.code);
  return new DaemonError({
    operation,
    reason,
    message: error.rawMessage,
    files: detail?.files ?? [],
  });
};
