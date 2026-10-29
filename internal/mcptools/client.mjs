import { readFileSync } from "node:fs";

export class AgxError extends Error {
  constructor(message) {
    super(message);
    this.name = "AgxError";
  }
}

// target reads the daemon's discovery file on every call, so a restarted daemon is found, and
// picks the credential: the launch's grant inside a launch, the management token outside one.
function target(server) {
  let daemon;
  try {
    daemon = JSON.parse(readFileSync(discovery, "utf8"));
  } catch {
    throw new AgxError("the agx daemon is not running; run agx daemon start");
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
    throw new AgxError(`cannot reach the agx daemon: ${error?.message ?? error}`);
  }
  const text = await response.text();
  let message = {};
  try {
    message = JSON.parse(text);
  } catch {
    // An error without a Connect body is described by its status.
  }
  if (!response.ok) {
    throw new AgxError(message.message ?? `HTTP ${response.status}`);
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
