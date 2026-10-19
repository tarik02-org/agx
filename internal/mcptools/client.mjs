import { readFileSync } from "node:fs";

// Reasons of agx.v1.ErrorDetail, as agx's JSON error codes.
const reasons = {
  1: "invalid_request",
  2: "not_found",
  3: "conflict",
  4: "failed_precondition",
  5: "needs_login",
  6: "config_invalid",
  7: "daemon_unavailable",
  8: "daemon_unavailable",
  9: "timeout",
  10: "internal",
  11: "mcp_failed",
};

export class AgxError extends Error {
  constructor(code, message) {
    super(message);
    this.name = "AgxError";
    this.code = code;
  }
}

// target reads the daemon's discovery file on every call, so a restarted daemon is found, and
// picks the credential: the launch's grant inside a launch, the management token outside one.
function target(server) {
  let daemon;
  try {
    daemon = JSON.parse(readFileSync(discovery, "utf8"));
  } catch {
    throw new AgxError("daemon_unavailable", "the agx daemon is not running; run agx daemon start");
  }
  const launchId = process.env.AGX_LAUNCH_ID;
  const grant = process.env.AGX_MCP_GRANT;
  if (launchId && grant && !all) {
    return { url: daemon.url, token: grant, scope: { launchId } };
  }
  const environmentInputs = {};
  for (const name of environment[server] ?? []) {
    if (process.env[name] !== undefined) environmentInputs[name] = process.env[name];
  }
  return { url: daemon.url, token: daemon.token, scope: { directory: process.cwd(), all, environmentInputs } };
}

// reason decodes the ErrorDetail of a Connect error: field 1, a varint.
function reason(error) {
  for (const detail of error.details ?? []) {
    if (detail.type !== "agx.v1.ErrorDetail") continue;
    const bytes = atob(detail.value ?? "");
    if (bytes.length >= 2 && bytes.charCodeAt(0) === 0x08) {
      return reasons[bytes.charCodeAt(1)] ?? "internal";
    }
  }
  switch (error.code) {
    case "unavailable":
    case "unauthenticated":
      return "daemon_unavailable";
    case "deadline_exceeded":
      return "timeout";
    case "canceled":
      return "interrupted";
  }
  return "internal";
}

export async function call(server, tool, args = {}, options = {}) {
  const { url, token, scope } = target(server);
  const headers = {
    "content-type": "application/json",
    "connect-protocol-version": "1",
    authorization: `Bearer ${token}`,
  };
  if (options.timeoutMs) headers["connect-timeout-ms"] = String(Math.ceil(options.timeoutMs));
  const body = JSON.stringify({ scope, mcpId: server, tool, argumentsJson: JSON.stringify(args ?? {}) });
  let response;
  try {
    response = await fetch(`${url}/agx.v1.RuntimeService/CallTool`, {
      method: "POST",
      headers,
      body,
      signal: options.signal,
    });
  } catch (error) {
    if (error?.name === "AbortError") throw error;
    throw new AgxError("daemon_unavailable", `cannot reach the agx daemon: ${error?.message ?? error}`);
  }
  const text = await response.text();
  let message = {};
  try {
    message = JSON.parse(text);
  } catch {
    // An error without a Connect body is described by its status.
  }
  if (!response.ok) {
    throw new AgxError(reason(message), message.message ?? `HTTP ${response.status}`);
  }
  const content = JSON.parse(message.contentJson || "[]");
  return {
    isError: message.isError ?? false,
    content,
    structuredContent: message.structuredContentJson ? JSON.parse(message.structuredContentJson) : undefined,
    text: content
      .filter((item) => item.type === "text")
      .map((item) => item.text)
      .join("\n"),
  };
}

function server(id, tools) {
  const out = {};
  for (const tool of tools) out[tool] = (args, options) => call(id, tool, args, options);
  return out;
}
