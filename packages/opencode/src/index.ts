import { define, type Context } from "@opencode/plugin/effect/plugin";
import type { SessionRequest } from "@opencode/plugin/effect/session";
import * as Effect from "effect/Effect";
import * as Layer from "effect/Layer";
import * as Path from "effect/Path";
import * as Schedule from "effect/Schedule";
import * as Schema from "effect/Schema";

import { Configuration } from "./Configuration.ts";
import { IntegrationError, type IntegrationFailure } from "./error.ts";
import { acquireLease } from "./lease.ts";
import { runtime } from "./runtime.ts";
import { Options } from "./schema.ts";

const register = Effect.fn("register")(function* (ctx: Context) {
  const configuration = yield* Configuration;
  const path = yield* Path.Path;
  const reload = Effect.all([ctx.skill.reload(), ctx.mcp.reload()]).pipe(Effect.asVoid);
  const lease = yield* acquireLease(reload);
  yield* ctx.skill.transform((editor) => {
    for (const skill of lease.current().skills) {
      editor.add(skill);
    }
  });
  yield* ctx.mcp.transform((editor) => {
    const active = lease.current();
    for (const endpoint of active.endpoints) {
      editor.set(endpoint.name, {
        type: "remote",
        url: endpoint.url.href,
        oauth: false,
        headers: { Authorization: `Bearer ${active.prepared.grant}` },
        ...(configuration.codemode === null ? {} : { codemode: configuration.codemode }),
      });
    }
  });
  const instructions = Effect.fn("instructions")(
    function* (event: SessionRequest) {
      yield* lease.ensure;
      const text = lease.current().prepared.content.instructions;
      event.system = event.system.filter((part) => part.metadata?.["agx.instructions"] !== true);
      if (text !== "") {
        event.system.push({ type: "text", text, metadata: { "agx.instructions": true } });
      }
    },
    Effect.catchTags({
      ConnectionError: (error) => Effect.die(error),
      DaemonError: (error) => Effect.die(error),
      IntegrationError: (error) => Effect.die(error),
    }),
  );
  yield* ctx.session.hook("context", instructions);
  yield* ctx.session.hook("compaction", instructions);
  yield* ctx.session.hook("generate", instructions);
  yield* ctx.session.hook("title", instructions);
  yield* ctx.shell.hook("create.before", (event) =>
    lease.ensure.pipe(
      Effect.andThen(
        Effect.sync(() => {
          const active = lease.current();
          const names = [
            ...Object.keys(active.environmentInputs),
            "AGX_MANAGEMENT_TOKEN",
            "AGX_LIFECYCLE_TOKEN",
            "AGX_LAUNCH_ID",
            "AGX_MCP_GRANT",
            "AGX_CONFIG",
          ];
          const protectedNames = new Set(
            names.map((name) => (path.sep === "\\" ? name.toUpperCase() : name)),
          );
          for (const name of names) {
            event.env[name] = undefined;
          }
          for (const name of Object.keys(event.env)) {
            if (protectedNames.has(path.sep === "\\" ? name.toUpperCase() : name)) {
              event.env[name] = undefined;
            }
          }
          event.env["AGX_LAUNCH_ID"] = active.prepared.launch.id;
          event.env["AGX_MCP_GRANT"] = active.prepared.grant;
          event.env["AGX_CONFIG"] = active.configuration;
        }),
      ),
      Effect.catchTags({
        ConnectionError: (error) => Effect.die(error),
        DaemonError: (error) => Effect.die(error),
        IntegrationError: (error) => Effect.die(error),
      }),
    ),
  );
  yield* ctx.command.transform((editor) =>
    editor.add({
      name: "agx-reload",
      description: "Reload this location's AGX configuration",
      execute: () => lease.refresh,
    }),
  );
  let warned = false;
  const warn = (error: IntegrationFailure) => {
    if (warned) {
      return Effect.void;
    }
    warned = true;
    return Effect.logWarning(error.message);
  };
  yield* lease.ensure.pipe(
    Effect.tap(() =>
      Effect.sync(() => {
        warned = false;
      }),
    ),
    Effect.catchTags({
      ConnectionError: warn,
      DaemonError: warn,
      IntegrationError: warn,
    }),
    Effect.repeat(Schedule.spaced("1 second")),
    Effect.forkScoped,
  );
  yield* Effect.logInfo("AGX resources ready", {
    skills: lease.current().skills.length,
    mcp: lease.current().prepared.mcp.length,
  });
});

export default define({
  id: "agx",
  effect: Effect.fn("agx.plugin")(
    function* (ctx) {
      const options = yield* Schema.decodeUnknownEffect(Options)(ctx.options).pipe(
        Effect.catchTag("SchemaError", (cause) =>
          Effect.fail(
            new IntegrationError({ operation: "plugin options", reason: "options", cause }),
          ),
        ),
      );
      const { harness, scopeDirectory, mcpDirectory, codemode, ...connection } = options;
      const configuration = Configuration.of({
        connection,
        harness: harness ?? "opencode",
        scopeDirectory: scopeDirectory ?? ctx.location.directory,
        mcpDirectory: mcpDirectory ?? ctx.location.directory,
        codemode: codemode ?? null,
      });
      const services = yield* Layer.build(runtime(configuration));
      yield* register(ctx).pipe(Effect.provide(services));
    },
    Effect.catchTags({
      ConnectionError: (error) => Effect.die(error),
      DaemonError: (error) => Effect.die(error),
      IntegrationError: (error) => Effect.die(error),
    }),
  ),
});
