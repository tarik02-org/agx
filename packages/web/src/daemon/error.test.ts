import { create } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { ErrorDetailSchema } from "agx-client/protocol";
import { describe, expect, it } from "vite-plus/test";

import { daemonError } from "~/daemon/error.ts";

describe("daemonError", () => {
  it("takes the reason from the Connect code", () => {
    const error = daemonError(
      "updateConfiguration",
      new ConnectError("local configuration changed; retry the command", Code.Aborted),
    );
    expect(error.reason).toBe("conflict");
    expect(error.message).toBe("local configuration changed; retry the command");
    expect(error.operation).toBe("updateConfiguration");
  });

  it("tells invalid configuration by its detail", () => {
    const cause = new ConnectError(
      "daemon configuration invalid",
      Code.FailedPrecondition,
      undefined,
      [{ desc: ErrorDetailSchema, value: create(ErrorDetailSchema, {}) }],
    );
    expect(daemonError("resolve", cause).reason).toBe("config_invalid");
  });

  it("tells an MCP server's login from the page's session", () => {
    expect(
      daemonError(
        "callTool",
        new ConnectError("MCP server docs needs a login", Code.Unauthenticated),
      ).reason,
    ).toBe("needs_login");
    expect(
      daemonError(
        "callTool",
        new ConnectError("management credential or launch grant required", Code.Unauthenticated),
      ).reason,
    ).toBe("unauthenticated");
    expect(
      daemonError("listTools", new ConnectError("MCP server docs: exited", Code.FailedPrecondition))
        .reason,
    ).toBe("mcp_failed");
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
