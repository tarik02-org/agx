import assert from "node:assert/strict";
import { execFile, spawn } from "node:child_process";
import { once } from "node:events";
import { mkdtemp, mkdir, readFile, realpath, rm, stat, writeFile } from "node:fs/promises";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import path from "node:path";
import { promisify } from "node:util";
import { fileURLToPath } from "node:url";
import { setTimeout } from "node:timers/promises";
import { test } from "node:test";
import { OpenCode } from "@opencode/client";
import * as Schema from "effect/Schema";
import { Document, Scalar } from "yaml";

const exec = promisify(execFile);
const repository = fileURLToPath(new URL("../../../", import.meta.url));
const Address = Schema.Struct({ port: Schema.Int });
const Discovery = Schema.Struct({
  url: Schema.String,
  token: Schema.String,
  pid: Schema.Int,
  instance_id: Schema.String,
});
const Bootstrap = Schema.Struct({ data: Schema.Struct({ discovery: Schema.String }) });
const Runtime = Schema.Struct({
  runtime: Schema.Struct({
    launches: Schema.optionalKey(
      Schema.Array(
        Schema.Struct({
          id: Schema.String,
          directory: Schema.String,
          kind: Schema.String,
          state: Schema.String,
        }),
      ),
    ),
  }),
});
const ShellEnvironment = Schema.Struct({
  secret: Schema.optionalKey(Schema.String),
  management: Schema.optionalKey(Schema.String),
  lifecycle: Schema.optionalKey(Schema.String),
  launch: Schema.String,
  grant: Schema.String,
});
const ModelRequest = Schema.Struct({
  stream: Schema.optionalKey(Schema.Boolean),
  messages: Schema.Array(
    Schema.Struct({ role: Schema.String, content: Schema.optionalKey(Schema.Json) }),
  ),
  tools: Schema.optionalKey(
    Schema.Array(Schema.Struct({ function: Schema.Struct({ name: Schema.String }) })),
  ),
});
type ModelRequest = typeof ModelRequest.Type;

test(
  "native OpenCode plugin consumes AGX resources and recovers after daemon replacement",
  { timeout: 180_000 },
  async (t) => {
    const root = await realpath(await mkdtemp(path.join(tmpdir(), "agx-opencode-e2e-")));
    const binary = path.join(root, process.platform === "win32" ? "agx.exe" : "agx");
    const daemon = path.join(root, process.platform === "win32" ? "agxd.exe" : "agxd");
    const mcp = path.join(root, process.platform === "win32" ? "fakemcp.exe" : "fakemcp");
    const alpha = path.join(root, "alpha");
    const beta = path.join(root, "beta");
    const invalid = path.join(root, "invalid");
    const invalidSkill = path.join(root, "invalid-skill");
    await Promise.all([mkdir(alpha), mkdir(beta), mkdir(invalid), mkdir(invalidSkill)]);
    await writeFile(
      path.join(invalidSkill, "SKILL.md"),
      "---\nname: invalid\ndescription: Invalid skill.\n---\nInvalid.",
    );
    if (process.platform !== "win32")
      await writeFile(path.join(invalidSkill, "bad:name"), "invalid resource path");
    const inherited = Object.fromEntries(
      Object.entries(process.env).filter(
        ([name]) => !/^(AGX_|OPENCODE_|XDG_|OPENAI_|ANTHROPIC_)/.test(name),
      ),
    );
    const env = {
      ...inherited,
      AGX_CONFIG: path.join(root, "agx.yaml"),
      AGX_CACHE: path.join(root, "cache"),
      AGX_STATE: path.join(root, "state"),
      AGX_DAEMON: daemon,
      AGX_MANAGEMENT_TOKEN: "must-not-reach-shell",
      AGX_LIFECYCLE_TOKEN: "must-not-reach-shell",
      MCP_INPUT_SECRET: "fixture-input",
      OPENCODE_TEST_HOME: path.join(root, "home"),
      OPENCODE_CONFIG_DIR: path.join(root, "opencode"),
      XDG_CONFIG_HOME: path.join(root, "xdg-config"),
      XDG_DATA_HOME: path.join(root, "xdg-data"),
      XDG_STATE_HOME: path.join(root, "xdg-state"),
      XDG_CACHE_HOME: path.join(root, "xdg-cache"),
      OPENCODE_DISABLE_MODELS_FETCH: "1",
      OPENCODE_CONFIG_PROJECT_DISABLE: "1",
      OPENCODE_DISABLE_CLAUDE_CODE: "1",
      OPENCODE_DISABLE_FFF: "1",
      OPENCODE_PASSWORD: "fixture-password",
      NO_COLOR: "1",
    };
    t.after(async () => {
      await exec(binary, ["daemon", "stop", "--force"], { env }).catch(() => undefined);
      await rm(root, { recursive: true, force: true });
    });
    await exec("go", ["build", "-o", binary, "./cmd/agx"], { cwd: repository });
    await exec("go", ["build", "-o", daemon, "./cmd/agxd"], { cwd: repository });
    await exec("go", ["build", "-o", mcp, "./internal/e2e/testdata/fakemcp"], { cwd: repository });
    await writeFile(
      env.AGX_CONFIG,
      "fragments: [main.yaml]\ndaemon: {lease_seconds: 6, renew_seconds: 1}\n",
    );
    const fragment = {
      skills: {
        invalid: { path: invalidSkill },
        memory: { description: "Memory-only instructions", content: "Memory skill body." },
        assets: {
          description: "Asset-bearing instructions",
          content: "Run scripts/check.sh.",
          files: {
            "scripts/check.sh": { content: "#!/bin/sh\nprintf asset-ok", executable: true },
            "references/data.txt": { content: "Reference bytes." },
          },
        },
        hidden: { description: "Beta only", content: "Beta skill body." },
      },
      instructions: { alpha: { text: "AGX_ALPHA_SENTINEL" }, beta: { text: "AGX_BETA_SENTINEL" } },
      mcp: {
        "team/echo": { command: [mcp], environment: { ECHO_TOKEN: "!env MCP_INPUT_SECRET" } },
        deferred: { command: [mcp], load: "on_demand" },
      },
      scopes: [
        { directory: invalid, add: { skills: ["invalid"] } },
        {
          directory: alpha,
          add: {
            skills: ["memory", "assets"],
            instructions: ["alpha"],
            mcp: ["team/echo", "deferred"],
          },
        },
        { directory: beta, add: { skills: ["hidden"], instructions: ["beta"] } },
      ],
    };
    const writeFragment = async () => {
      const document = new Document(fragment);
      const variable = new Scalar("MCP_INPUT_SECRET");
      variable.tag = "!env";
      document.setIn(["mcp", "team/echo", "environment", "ECHO_TOKEN"], variable);
      return writeFile(path.join(root, "main.yaml"), String(document));
    };
    await writeFragment();

    const requests: ModelRequest[] = [];
    const model = createServer(async (request, response) => {
      const chunks: Buffer[] = [];
      for await (const chunk of request) chunks.push(Buffer.from(chunk));
      const body = Schema.decodeUnknownSync(Schema.fromJsonString(ModelRequest))(
        Buffer.concat(chunks).toString(),
      );
      requests.push(body);
      const tool = body.tools?.find((entry) => entry.function.name.includes("echo"));
      const lastUser = body.messages.findLastIndex((message) => message.role === "user");
      const called = body.messages.slice(lastUser + 1).some((message) => message.role === "tool");
      const delta =
        tool !== undefined && !called
          ? {
              role: "assistant",
              tool_calls: [
                {
                  index: 0,
                  id: "call_echo",
                  type: "function",
                  function: { name: tool.function.name, arguments: '{"text":"through-opencode"}' },
                },
              ],
            }
          : { role: "assistant", content: "fixture complete" };
      const finish = tool !== undefined && !called ? "tool_calls" : "stop";
      if (body.stream === false) {
        response.writeHead(200, { "content-type": "application/json" });
        response.end(
          JSON.stringify({
            id: "fixture",
            object: "chat.completion",
            created: 1,
            model: "fixture",
            choices: [{ index: 0, message: delta, finish_reason: finish }],
            usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 },
          }),
        );
        return;
      }
      response.writeHead(200, { "content-type": "text/event-stream" });
      for (const item of [
        { choices: [{ index: 0, delta, finish_reason: null }] },
        {
          choices: [{ index: 0, delta: {}, finish_reason: finish }],
          usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 },
        },
      ])
        response.write(
          `data: ${JSON.stringify({ id: "fixture", object: "chat.completion.chunk", created: 1, model: "fixture", ...item })}\n\n`,
        );
      response.end("data: [DONE]\n\n");
    });
    model.listen(0, "127.0.0.1");
    await once(model, "listening");
    t.after(() => new Promise<void>((resolve) => model.close(() => resolve())));
    const modelPort = Schema.decodeUnknownSync(Address)(model.address()).port;

    const reservation = createServer();
    reservation.listen(0, "127.0.0.1");
    await once(reservation, "listening");
    const port = Schema.decodeUnknownSync(Address)(reservation.address()).port;
    await new Promise<void>((resolve) => reservation.close(() => resolve()));
    const opencode = path.join(
      repository,
      "packages/opencode/node_modules/@opencode/cli/bin/opencode.exe",
    );
    const config = {
      plugins: [
        {
          package: path.join(repository, "packages/opencode"),
          options: { executable: binary, codemode: false },
        },
      ],
      providers: {
        fixture: {
          package: "@ai-sdk/openai-compatible",
          settings: { baseURL: `http://127.0.0.1:${modelPort}/v1`, apiKey: "fixture" },
          models: { fixture: { type: "chat", limit: { context: 32_000, output: 1024 } } },
        },
      },
    };
    await mkdir(env.OPENCODE_CONFIG_DIR, { recursive: true });
    await mkdir(env.OPENCODE_TEST_HOME, { recursive: true });
    await writeFile(path.join(env.OPENCODE_CONFIG_DIR, "opencode.json"), JSON.stringify(config));
    const server = spawn(
      opencode,
      [
        "serve",
        "--hostname",
        "127.0.0.1",
        "--port",
        String(port),
        "--log-level",
        "debug",
        "--print-logs",
      ],
      {
        cwd: alpha,
        env: { ...env, OPENCODE_CONFIG_CONTENT: JSON.stringify(config) },
        stdio: ["ignore", "pipe", "pipe"],
      },
    );
    let logs = "";
    server.stdout.on("data", (chunk: Buffer) => {
      logs += chunk.toString();
    });
    server.stderr.on("data", (chunk: Buffer) => {
      logs += chunk.toString();
    });
    t.after(async () => {
      if (server.exitCode === null) {
        server.kill("SIGTERM");
        await once(server, "exit");
      }
    });
    const api = OpenCode.make({
      baseUrl: `http://127.0.0.1:${port}`,
      headers: {
        Authorization: `Basic ${Buffer.from("opencode:fixture-password").toString("base64")}`,
      },
    });
    let ready = false;
    for (let attempt = 0; attempt < 100; attempt++) {
      try {
        await api.server.info();
        ready = true;
        break;
      } catch {
        await setTimeout(100);
      }
      if (server.exitCode !== null) break;
    }
    assert.ok(ready, `OpenCode did not start: ${logs}`);
    let plugin = await api.plugin.list({ location: { directory: alpha } });
    for (
      let attempt = 0;
      attempt < 100 && !plugin.data.some((entry) => entry.id === "agx");
      attempt++
    ) {
      await setTimeout(100);
      plugin = await api.plugin.list({ location: { directory: alpha } });
    }
    assert.ok(
      plugin.data.some((entry) => entry.id === "agx" && entry.state.status === "active"),
      `${JSON.stringify(plugin.data.filter((entry) => entry.source.type !== "builtin"))}\n${JSON.stringify(await api.config.get({ location: { directory: alpha } }))}\n${logs}`,
    );
    const skills = (await api.skill.list({ location: { directory: alpha } })).data.filter((entry) =>
      entry.id.startsWith("agx-"),
    );
    assert.deepEqual(skills.map((entry) => entry.name).sort(), ["assets", "memory"]);
    const memory = skills.find((entry) => entry.name === "memory");
    const asset = skills.find((entry) => entry.name === "assets");
    assert.ok(memory?.path.startsWith("skill://"));
    assert.ok(asset);
    assert.equal(
      await readFile(path.join(path.dirname(asset.path), "references/data.txt"), "utf8"),
      "Reference bytes.",
    );
    if (process.platform !== "win32")
      assert.notEqual(
        (await stat(path.join(path.dirname(asset.path), "scripts/check.sh"))).mode & 0o111,
        0,
      );
    for (let attempt = 0; attempt < 100; attempt++) {
      const otherPlugin = (await api.plugin.list({ location: { directory: beta } })).data.find(
        (entry) => entry.id === "agx",
      );
      if (otherPlugin?.state.status === "active") break;
      await setTimeout(100);
    }
    const otherSkills = (await api.skill.list({ location: { directory: beta } })).data.filter(
      (entry) => entry.id.startsWith("agx-"),
    );
    assert.deepEqual(
      otherSkills.map((entry) => entry.name),
      ["hidden"],
    );
    assert.equal(
      (await api.mcp.list({ location: { directory: alpha } })).data.filter((entry) =>
        entry.name.startsWith("agx-"),
      ).length,
      1,
    );

    const session = await api.session.create({
      location: { directory: alpha },
      model: { providerID: "fixture", id: "fixture" },
      permissions: [{ action: "*", resource: "*", effect: "allow" }],
    });
    await api.session.prompt({ sessionID: session.id, text: "Use the echo tool, then finish." });
    await api.experimental.session.wait({ sessionID: session.id });
    assert.ok(
      requests.some((request) =>
        request.messages.some(
          (message) =>
            message.role === "system" &&
            JSON.stringify(message.content).includes("AGX_ALPHA_SENTINEL"),
        ),
      ),
    );
    assert.ok(
      requests.some((request) =>
        request.messages.some(
          (message) =>
            message.role === "tool" &&
            JSON.stringify(message.content).includes("echo:through-opencode:"),
        ),
      ),
    );

    const connected = Schema.decodeUnknownSync(Schema.fromJsonString(Bootstrap))(
      (await exec(binary, ["--json", "daemon", "connect"], { env })).stdout,
    );
    const discovery = Schema.decodeUnknownSync(Schema.fromJsonString(Discovery))(
      await readFile(connected.data.discovery, "utf8"),
    );
    const readRuntime = async () => {
      const current = Schema.decodeUnknownSync(Schema.fromJsonString(Discovery))(
        await readFile(connected.data.discovery, "utf8"),
      );
      const response = await fetch(`${current.url}/agx.v1.RuntimeService/GetRuntime`, {
        method: "POST",
        headers: {
          "content-type": "application/json",
          Authorization: `Bearer ${current.token}`,
        },
        body: "{}",
      }).then((response) => response.text());
      const view = Schema.decodeUnknownSync(Schema.fromJsonString(Runtime))(response).runtime;
      return { ...view, launches: view.launches ?? [] };
    };
    assert.equal(
      (await readRuntime()).launches.filter(
        (entry) => entry.kind === "KIND_RESOURCES" && entry.state === "STATE_RUNNING",
      ).length,
      2,
    );
    if (process.platform !== "win32") {
      let failed = false;
      for (let attempt = 0; attempt < 100; attempt++) {
        const entry = (await api.plugin.list({ location: { directory: invalid } })).data.find(
          (item) => item.id === "agx",
        );
        if (entry?.state.status === "failed") {
          failed = true;
          break;
        }
        await setTimeout(100);
      }
      assert.ok(failed, "invalid resource paths must reject plugin activation");
      assert.ok(
        !(await readRuntime()).launches.some(
          (entry) => entry.directory === invalid && entry.state === "STATE_RUNNING",
        ),
        "failed content validation left a resource lease running",
      );
    }
    const environmentFile = path.join(root, "shell-env.json");
    const script = path.join(root, "shell-env.mjs");
    await writeFile(
      script,
      `import { writeFileSync } from 'node:fs'; writeFileSync(${JSON.stringify(environmentFile)}, JSON.stringify({
      secret: process.env.MCP_INPUT_SECRET, management: process.env.AGX_MANAGEMENT_TOKEN,
      lifecycle: process.env.AGX_LIFECYCLE_TOKEN, launch: process.env.AGX_LAUNCH_ID, grant: process.env.AGX_MCP_GRANT
    }));`,
    );
    await api.shell.create({
      location: { directory: alpha },
      command: `${JSON.stringify(process.execPath)} ${JSON.stringify(script)}`,
    });
    for (let attempt = 0; attempt < 100; attempt++) {
      if (
        await readFile(environmentFile, "utf8").then(
          () => true,
          () => false,
        )
      )
        break;
      await setTimeout(50);
    }
    const shell = Schema.decodeUnknownSync(Schema.fromJsonString(ShellEnvironment))(
      await readFile(environmentFile, "utf8"),
    );
    assert.equal(shell.secret, undefined);
    assert.equal(shell.management, undefined);
    assert.equal(shell.lifecycle, undefined);
    assert.ok(shell.launch !== "" && shell.grant !== "");
    await exec(binary, ["daemon", "restart"], { env });
    await setTimeout(1500);
    await api.session.generate({ sessionID: session.id, prompt: "Confirm recovery." });
    const beforeCrash = Schema.decodeUnknownSync(Schema.fromJsonString(Discovery))(
      await readFile(connected.data.discovery, "utf8"),
    );
    process.kill(beforeCrash.pid, "SIGKILL");
    let recovered = false;
    for (let attempt = 0; attempt < 100; attempt++) {
      await setTimeout(100);
      const text = await readFile(connected.data.discovery, "utf8").catch(() => null);
      if (text === null) continue;
      const next = Schema.decodeUnknownSync(Schema.fromJsonString(Discovery))(text);
      if (next.instance_id !== beforeCrash.instance_id) {
        recovered = true;
        break;
      }
    }
    assert.ok(recovered, "a crashed daemon was not restarted");
    await api.session.generate({ sessionID: session.id, prompt: "Confirm crash recovery." });
    await exec(binary, ["daemon", "stop"], { env });
    for (let attempt = 0; attempt < 100; attempt++) {
      if (
        !(await readFile(connected.data.discovery).then(
          () => true,
          () => false,
        ))
      )
        break;
      await setTimeout(50);
    }
    const blocker = createServer((_request, response) => {
      response.writeHead(503);
      response.end();
    });
    blocker.listen(Number(new URL(discovery.url).port), "127.0.0.1");
    await once(blocker, "listening");
    t.after(() => new Promise<void>((resolve) => blocker.close(() => resolve())));
    await exec(binary, ["daemon", "start"], { env });
    const moved = Schema.decodeUnknownSync(Schema.fromJsonString(Discovery))(
      await readFile(connected.data.discovery, "utf8"),
    );
    assert.notEqual(moved.url, discovery.url);
    await setTimeout(1500);
    const echoCompleted = (request: ModelRequest) =>
      request.messages
        .slice(request.messages.findLastIndex((message) => message.role === "user") + 1)
        .some(
          (message) =>
            message.role === "tool" &&
            JSON.stringify(message.content).includes("echo:through-opencode:"),
        );
    const priorCalls = requests.filter(echoCompleted).length;
    await api.session.prompt({
      sessionID: session.id,
      text: "Use echo again after the port change.",
    });
    await api.experimental.session.wait({ sessionID: session.id });
    assert.ok(requests.filter(echoCompleted).length > priorCalls);
    fragment.instructions.alpha.text = "AGX_RELOADED_SENTINEL";
    await writeFragment();
    await api.session.command({ sessionID: session.id, name: "agx-reload", text: "" });
    await api.session.generate({ sessionID: session.id, prompt: "Confirm reload." });
    assert.ok(
      requests.some((request) =>
        request.messages.some(
          (message) =>
            message.role === "system" &&
            JSON.stringify(message.content).includes("AGX_RELOADED_SENTINEL"),
        ),
      ),
    );
    const betaLease = (await readRuntime()).launches.find(
      (entry) => entry.directory === beta && entry.state === "STATE_RUNNING",
    );
    assert.ok(betaLease);
    await api.debug.location.evict({ location: { directory: beta } });
    assert.equal(
      (await readRuntime()).launches.find((entry) => entry.id === betaLease.id)?.state,
      "STATE_FINISHED",
    );
    assert.ok(!JSON.stringify(requests).includes(shell.grant), "MCP grants reached model context");
    await exec(binary, ["daemon", "stop", "--force"], { env });
    await setTimeout(1800);
    const stopped = (await exec(binary, ["--json", "daemon", "status"], { env })).stdout;
    assert.equal(
      Schema.decodeUnknownSync(Schema.fromJsonString(Schema.Struct({ data: Schema.Null })))(stopped)
        .data,
      null,
    );
    await exec(binary, ["daemon", "start"], { env });
    await setTimeout(1200);
    const requestsBeforeRevoked = requests.length;
    await assert.rejects(
      api.session.generate({ sessionID: session.id, prompt: "Do not bypass revocation." }),
    );
    assert.equal(requests.length, requestsBeforeRevoked, "a revoked lease reached the model");
    assert.equal(
      (await readRuntime()).launches.filter((entry) => entry.state === "STATE_RUNNING").length,
      0,
    );
  },
);
