import { create } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { describe, expect, it } from "vite-plus/test";

import { daemonError } from "~/daemon/error.ts";
import { ErrorDetail_Reason, ErrorDetailSchema } from "~/gen/agx/v1/agx_pb.ts";

describe("daemonError", () => {
  it("takes the reason from the daemon's error detail", () => {
    const cause = new ConnectError(
      "MCP server docs needs a login",
      Code.FailedPrecondition,
      undefined,
      [
        {
          desc: ErrorDetailSchema,
          value: create(ErrorDetailSchema, { reason: ErrorDetail_Reason.NEEDS_LOGIN }),
        },
      ],
    );
    const error = daemonError("callTool", cause);
    expect(error.reason).toBe("needs_login");
    expect(error.message).toBe("MCP server docs needs a login");
    expect(error.operation).toBe("callTool");
  });

  it("falls back to the Connect code without a detail", () => {
    expect(
      daemonError(
        "watchRuntime",
        new ConnectError("management credential required", Code.Unauthenticated),
      ).reason,
    ).toBe("unauthenticated");
  });

  it("treats network failures as an unreachable daemon", () => {
    expect(daemonError("watchRuntime", new TypeError("Failed to fetch")).reason).toBe(
      "unreachable",
    );
  });

  it("keeps an error the daemon answered with apart from an unreachable daemon", () => {
    expect(
      daemonError("resolve", new ConnectError("open agx.yaml: denied", Code.Unknown)).reason,
    ).toBe("internal");
    expect(
      daemonError("resolve", new ConnectError("the operation was aborted", Code.Canceled)).reason,
    ).toBe("canceled");
  });
});
