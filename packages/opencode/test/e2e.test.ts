import assert from "node:assert/strict";
import { execFile, spawn } from "node:child_process";
import { once } from "node:events";
import { cp, mkdtemp, mkdir, readFile, realpath, rm, stat, writeFile } from "node:fs/promises";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import { setTimeout } from "node:timers/promises";
import { fileURLToPath, pathToFileURL } from "node:url";
import { promisify } from "node:util";

import { ClientError, OpenCode } from "@opencode/client";
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
const SessionCatalog = Schema.Struct({
  data: Schema.Struct({
    sessions: Schema.NullOr(
      Schema.Array(
        Schema.Struct({
          id: Schema.String,
          directory: Schema.String,
          harness: Schema.String,
          title: Schema.optionalKey(Schema.String),
          sourceId: Schema.optionalKey(Schema.String),
          scopeId: Schema.optionalKey(Schema.String),
          online: Schema.optionalKey(Schema.Boolean),
        }),
      ),
    ),
  }),
});
const MissingFile = Schema.Struct({ code: Schema.Literal("ENOENT") });
const ServerStarting = Schema.Struct({
  reason: Schema.Literal("UnexpectedStatus"),
  cause: Schema.Struct({ status: Schema.Literal(503) }),
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

await test(
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
    if (process.platform !== "win32") {
      await writeFile(path.join(invalidSkill, "bad:name"), "invalid resource path");
    }
    const inherited = Object.fromEntries(
      Object.entries(process.env).filter(
        ([name]) => !/^(AGX_|OPENCODE_|XDG_|OPENAI_|ANTHROPIC_|NPM_CONFIG_|GIT_)/i.test(name),
      ),
    );
    const env = {
      ...inherited,
      AGX_CONFIG: path.join(root, "agx.yaml"),
      AGX_CACHE: path.join(root, "cache"),
      AGX_STATE: path.join(root, "state"),
      AGX_DAEMON: daemon,
      CODEX_HOME: path.join(root, "other-native-stores/codex"),
      CLAUDE_CONFIG_DIR: path.join(root, "other-native-stores/claude"),
      PI_CODING_AGENT_DIR: path.join(root, "other-native-stores/pi"),
      PI_CODING_AGENT_SESSION_DIR: path.join(root, "other-native-stores/pi-sessions"),
      AGX_MANAGEMENT_TOKEN: "must-not-reach-shell",
      AGX_LIFECYCLE_TOKEN: "must-not-reach-shell",
      MCP_INPUT_SECRET: "fixture-input",
      GIT_CONFIG_GLOBAL: path.join(root, "gitconfig"),
      GIT_CONFIG_NOSYSTEM: "1",
      NPM_CONFIG_CACHE: path.join(root, "npm-cache"),
      NPM_CONFIG_USERCONFIG: path.join(root, "npmrc"),
      NPM_CONFIG_GLOBALCONFIG: path.join(root, "global-npmrc"),
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
      try {
        await exec(binary, ["daemon", "stop", "--force"], { env });
      } catch (error) {
        if (!Schema.is(MissingFile)(error)) {
          throw error;
        }
      } finally {
        await rm(root, { recursive: true, force: true });
      }
    });
    await exec("go", ["build", "-o", binary, "./cmd/agx"], { cwd: repository });
    await exec("go", ["build", "-o", daemon, "./cmd/agxd"], { cwd: repository });
    await exec("go", ["build", "-o", mcp, "./internal/e2e/testdata/fakemcp"], { cwd: repository });
    await writeFile(
      env.AGX_CONFIG,
      "fragments: [main.yaml]\ndaemon: {lease_seconds: 6, renew_seconds: 1}\n",
    );
    const fragment = {
      harnesses: { "opencode-fixture": { base: "opencode" } },
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
      agents: {
        helper: {
          description: "Alpha only helper",
          content: "Helper prompt.",
          harnesses: { opencode: { steps: 7 }, claude: { model: "ignored" } },
        },
      },
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
            agents: ["helper"],
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
    const model = createServer((request, response) => {
      let text = "";
      request.setEncoding("utf8");
      request.on("data", (chunk: unknown) => {
        text += Schema.decodeUnknownSync(Schema.String)(chunk);
      });
      request.on("end", () => {
        const body = Schema.decodeUnknownSync(Schema.fromJsonString(ModelRequest))(text);
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
                    function: {
                      name: tool.function.name,
                      arguments: '{"text":"through-opencode"}',
                    },
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
        ]) {
          response.write(
            `data: ${JSON.stringify({ id: "fixture", object: "chat.completion.chunk", created: 1, model: "fixture", ...item })}\n\n`,
          );
        }
        response.end("data: [DONE]\n\n");
      });
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
    const pluginRepository = path.join(root, "plugin-release");
    await cp(path.join(repository, "packages/opencode/dist/git"), pluginRepository, {
      recursive: true,
    });
    await exec("git", ["init", "--initial-branch=release", pluginRepository], { env });
    await exec("git", ["-C", pluginRepository, "add", "."], { env });
    const commitPlugin = (message: string) =>
      exec(
        "git",
        [
          "-C",
          pluginRepository,
          "-c",
          `core.hooksPath=${path.join(root, "empty-hooks")}`,
          "-c",
          "commit.gpgSign=false",
          "-c",
          "user.name=agx fixture",
          "-c",
          "user.email=fixture@example.invalid",
          "commit",
          "-am",
          message,
        ],
        { env },
      );
    await commitPlugin("fixture plugin release");
    const pluginTarget = `git+${pathToFileURL(pluginRepository).href}#release`;
    await mkdir(env.OPENCODE_CONFIG_DIR, { recursive: true });
    await mkdir(env.OPENCODE_TEST_HOME, { recursive: true });
    await exec(opencode, ["plugin", "add", pluginTarget], { cwd: alpha, env });
    const installed = Schema.decodeUnknownSync(
      Schema.fromJsonString(
        Schema.Struct({
          plugins: Schema.Array(Schema.String),
        }),
      ),
    )(await readFile(path.join(env.OPENCODE_CONFIG_DIR, "opencode.json"), "utf8"));
    assert.ok(
      installed.plugins.includes(pluginTarget),
      "plugin add did not register the Git package",
    );
    const catalogFile = path.join(root, "tool-catalog.json");
    const catalogPlugin = path.join(root, "catalog-plugin");
    await mkdir(catalogPlugin);
    await writeFile(
      path.join(catalogPlugin, "package.json"),
      JSON.stringify({
        name: "agx-fixture-catalog",
        version: "0.0.0",
        type: "module",
        exports: "./index.mjs",
      }),
    );
    await writeFile(
      path.join(catalogPlugin, "index.mjs"),
      `import { writeFileSync } from "node:fs";
import { define } from "@opencode/plugin/effect/plugin";
import * as Effect from "effect/Effect";

export default define({
  id: "agx.fixture.catalog",
  effect: Effect.fn("fixture.catalog")(function* (ctx) {
    yield* ctx.command.transform((editor) => editor.add({
      name: "fixture-catalog",
      description: "Read the native tool catalog",
      execute: Effect.fn("fixture.catalog.read")(function* () {
        const tools = yield* ctx.tool.list();
        yield* Effect.sync(() => writeFileSync(${JSON.stringify(catalogFile)}, JSON.stringify(tools.map((tool) => tool.id))));
      }),
    }));
  }),
});`,
    );
    const config = {
      plugins: [
        { package: catalogPlugin },
        {
          package: pluginTarget,
          options: {
            executable: binary,
            codemode: false,
            harness: "opencode-fixture",
          },
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
      } catch (error) {
        if (
          !Schema.is(Schema.instanceOf(ClientError))(error) ||
          (error.reason !== "Transport" && !Schema.is(ServerStarting)(error))
        ) {
          throw error;
        }
        await setTimeout(100);
      }
      if (server.exitCode !== null) {
        break;
      }
    }
    assert.ok(ready, `OpenCode did not start: ${logs}`);
    let plugin = await api.plugin.list({ location: { directory: alpha } });
    for (
      let attempt = 0;
      attempt < 100 &&
      !["agx", "agx.fixture.catalog"].every((id) =>
        plugin.data.some((entry) => entry.id === id && entry.state.status === "active"),
      );
      attempt++
    ) {
      await setTimeout(100);
      plugin = await api.plugin.list({ location: { directory: alpha } });
    }
    assert.ok(
      plugin.data.some((entry) => entry.id === "agx" && entry.state.status === "active"),
      `${JSON.stringify(plugin.data.filter((entry) => entry.source.type !== "builtin"))}\n${JSON.stringify(await api.config.get({ location: { directory: alpha } }))}\n${logs}`,
    );
    assert.ok(
      plugin.data.some(
        (entry) => entry.id === "agx.fixture.catalog" && entry.state.status === "active",
      ),
      `${JSON.stringify(plugin.data.filter((entry) => entry.source.type !== "builtin"))}\n${logs}`,
    );
    assert.ok(
      plugin.data.some((entry) => entry.id === "agx" && entry.source.type === "package"),
      "AGX must load from OpenCode's installed Git package",
    );
    const skills = (await api.skill.list({ location: { directory: alpha } })).data.filter((entry) =>
      entry.id.startsWith("agx-"),
    );
    assert.deepEqual(skills.map((entry) => entry.name).toSorted(), ["assets", "memory"]);
    const memory = skills.find((entry) => entry.name === "memory");
    const asset = skills.find((entry) => entry.name === "assets");
    assert.ok(memory?.path.startsWith("skill://"));
    assert.ok(asset);
    assert.equal(
      await readFile(path.join(path.dirname(asset.path), "references/data.txt"), "utf8"),
      "Reference bytes.",
    );
    if (process.platform !== "win32") {
      assert.notEqual(
        (await stat(path.join(path.dirname(asset.path), "scripts/check.sh"))).mode & 0o111,
        0,
      );
    }
    const helper = (await api.agent.list({ location: { directory: alpha } })).data.find(
      (entry) => entry.id === "helper",
    );
    assert.equal(helper?.description, "Alpha only helper");
    assert.equal(helper?.system, "Helper prompt.");
    assert.equal(helper?.mode, "subagent");
    assert.equal(helper?.steps, 7);
    for (let attempt = 0; attempt < 100; attempt++) {
      const otherPlugin = (await api.plugin.list({ location: { directory: beta } })).data.find(
        (entry) => entry.id === "agx",
      );
      if (otherPlugin?.state.status === "active") {
        break;
      }
      await setTimeout(100);
    }
    const otherSkills = (await api.skill.list({ location: { directory: beta } })).data.filter(
      (entry) => entry.id.startsWith("agx-"),
    );
    assert.deepEqual(
      otherSkills.map((entry) => entry.name),
      ["hidden"],
    );
    assert.ok(
      !(await api.agent.list({ location: { directory: beta } })).data.some(
        (entry) => entry.id === "helper",
      ),
    );
    assert.equal(
      (await api.mcp.list({ location: { directory: alpha } })).data.filter((entry) =>
        entry.name.startsWith("agx-"),
      ).length,
      1,
    );
    let nativeServers = (await api.mcp.list({ location: { directory: alpha } })).data.filter(
      (entry) => entry.name.startsWith("agx-"),
    );
    for (
      let attempt = 0;
      attempt < 100 && !nativeServers.every((entry) => entry.status.status === "connected");
      attempt++
    ) {
      await setTimeout(100);
      nativeServers = (await api.mcp.list({ location: { directory: alpha } })).data.filter(
        (entry) => entry.name.startsWith("agx-"),
      );
    }
    assert.ok(
      nativeServers.every((entry) => entry.status.status === "connected"),
      JSON.stringify(nativeServers),
    );

    const session = await api.session.create({
      location: { directory: alpha },
      model: { providerID: "fixture", id: "fixture" },
      permissions: [{ action: "*", resource: "*", effect: "allow" }],
    });
    const betaSession = await api.session.create({
      location: { directory: beta },
      title: "Beta indexed session",
    });
    const readSessions = async () => {
      const output = (await exec(binary, ["--json", "sessions", "--all"], { env, cwd: alpha }))
        .stdout;
      assert.ok(
        !output.includes(env.OPENCODE_PASSWORD),
        "native credentials entered the public session catalog",
      );
      return (
        Schema.decodeUnknownSync(Schema.fromJsonString(SessionCatalog))(output).data.sessions ?? []
      );
    };
    let indexed = await readSessions();
    for (
      let attempt = 0;
      attempt < 100 && !indexed.some((entry) => entry.id === session.id && entry.online === true);
      attempt++
    ) {
      await setTimeout(100);
      indexed = await readSessions();
    }
    const alphaSource = indexed.find((entry) => entry.id === session.id);
    assert.ok(
      alphaSource?.sourceId !== undefined &&
        alphaSource.scopeId !== undefined &&
        alphaSource.online === true,
      "native session was not published to AGX",
    );
    for (
      let attempt = 0;
      attempt < 100 && !indexed.some((entry) => entry.id === betaSession.id);
      attempt++
    ) {
      await setTimeout(100);
      indexed = await readSessions();
    }
    const betaSource = indexed.find((entry) => entry.id === betaSession.id);
    assert.ok(betaSource?.scopeId !== undefined && betaSource.online === true);
    assert.equal(
      betaSource.sourceId,
      alphaSource.sourceId,
      "one native profile published inconsistent source identities",
    );
    assert.notEqual(betaSource.scopeId, alphaSource.scopeId);
    assert.equal(alphaSource.harness, "opencode-fixture");
    assert.equal(betaSource.harness, "opencode-fixture");
    await api.session.update({ sessionID: betaSession.id, title: "Beta renamed session" });
    for (
      let attempt = 0;
      attempt < 100 &&
      !indexed.some(
        (entry) => entry.id === betaSession.id && entry.title === "Beta renamed session",
      );
      attempt++
    ) {
      await setTimeout(100);
      indexed = await readSessions();
    }
    assert.ok(
      indexed.some(
        (entry) => entry.id === betaSession.id && entry.title === "Beta renamed session",
      ),
    );
    // MCP connection status precedes OpenCode's debounced native tool registration.
    let catalog: ReadonlyArray<string> = [];
    for (let attempt = 0; attempt < 100; attempt++) {
      await api.session.command({ sessionID: session.id, name: "fixture-catalog", text: "" });
      catalog = Schema.decodeUnknownSync(Schema.fromJsonString(Schema.Array(Schema.String)))(
        await readFile(catalogFile, "utf8"),
      );
      if (catalog.some((id) => id.startsWith("agx-") && id.endsWith("echo"))) {
        break;
      }
      await setTimeout(50);
    }
    assert.ok(
      catalog.some((id) => id.startsWith("agx-") && id.endsWith("echo")),
      `AGX echo did not reach OpenCode's tool catalog: ${JSON.stringify(catalog)}`,
    );
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
      JSON.stringify(
        requests.map((request) => ({ tools: request.tools, messages: request.messages.slice(-4) })),
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
      const result = await fetch(`${current.url}/agx.v1.RuntimeService/GetRuntime`, {
        method: "POST",
        headers: {
          "content-type": "application/json",
          Authorization: `Bearer ${current.token}`,
        },
        body: "{}",
      }).then((response) => response.text());
      const view = Schema.decodeUnknownSync(Schema.fromJsonString(Runtime))(result).runtime;
      return { ...view, launches: view.launches ?? [] };
    };
    const childSession = await api.session.create({
      location: { directory: alpha },
      parentID: session.id,
    });
    const archivedSession = await api.session.create({
      location: { directory: alpha },
      title: "Archived session",
    });
    await api.session.update({ sessionID: archivedSession.id, archived: true });
    await setTimeout(500);
    indexed = await readSessions();
    assert.ok(
      !indexed.some((entry) => entry.id === childSession.id || entry.id === archivedSession.id),
      "non-listable native sessions entered the launcher catalog",
    );
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
          (error: unknown) => {
            if (Schema.is(MissingFile)(error)) {
              return false;
            }
            throw error;
          },
        )
      ) {
        break;
      }
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
      const text = await readFile(connected.data.discovery, "utf8").catch((error: unknown) => {
        if (Schema.is(MissingFile)(error)) {
          return null;
        }
        throw error;
      });
      if (text === null) {
        continue;
      }
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
          (error: unknown) => {
            if (Schema.is(MissingFile)(error)) {
              return false;
            }
            throw error;
          },
        ))
      ) {
        break;
      }
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
    indexed = await readSessions();
    for (
      let attempt = 0;
      attempt < 100 &&
      !indexed.some((entry) => entry.id === betaSession.id && entry.online !== true);
      attempt++
    ) {
      await setTimeout(100);
      indexed = await readSessions();
    }
    assert.ok(
      indexed.some((entry) => entry.id === betaSession.id && entry.online !== true),
      "location disposal discarded offline native history",
    );
    await writeFile(path.join(pluginRepository, "README.md"), "Updated fixture Git release.\n");
    await commitPlugin("fixture plugin update");
    const available = await api.plugin.check({
      location: { directory: alpha },
      target: pluginTarget,
    });
    assert.ok(
      available.data.some(
        (entry) =>
          entry.id === "agx" && entry.source.type === "package" && entry.source.outdated === true,
      ),
      "OpenCode did not discover the new Git revision",
    );
    await api.plugin.update({ location: { directory: alpha }, targets: [pluginTarget] });
    const updated = await api.plugin.check({
      location: { directory: alpha },
      target: pluginTarget,
    });
    assert.ok(
      updated.data.some(
        (entry) =>
          entry.id === "agx" &&
          entry.state.status === "active" &&
          entry.source.type === "package" &&
          entry.source.outdated === undefined,
      ),
      "OpenCode did not activate the Git update",
    );
    await api.session.generate({ sessionID: session.id, prompt: "Confirm plugin update." });
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
