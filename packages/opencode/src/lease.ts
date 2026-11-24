import { Code } from "@connectrpc/connect";
import type { Skill } from "@opencode/schema/skill";
import { AgxClient } from "agx-client/AgxClient";
import { AgxConnection } from "agx-client/AgxConnection";
import type { DaemonError } from "agx-client/error";
import type { Discovery } from "agx-client/schema";
import * as Clock from "effect/Clock";
import * as Config from "effect/Config";
import * as Crypto from "effect/Crypto";
import * as Effect from "effect/Effect";
import * as Hex from "effect/encoding/Hex";
import * as Option from "effect/Option";
import * as Schedule from "effect/Schedule";
import * as Schema from "effect/Schema";
import * as Semaphore from "effect/Semaphore";
import * as Stream from "effect/Stream";

import { Configuration } from "./Configuration.ts";
import { IntegrationError, type IntegrationFailure } from "./error.ts";
import {
  LeaseEnvelope,
  type LeaseResources,
  Prepared,
  ResourceWatchEvent,
  Snapshot,
} from "./schema.ts";
import { SkillDeployer } from "./SkillDeployer.ts";

export interface ResourceChanges {
  readonly skills: boolean;
  readonly mcp: boolean;
}

interface Applied {
  readonly resources: LeaseResources;
  readonly skills: ReadonlyArray<Skill.Info>;
  readonly endpoints: ReadonlyArray<{ readonly name: string; readonly url: URL }>;
}

// One lease for the plugin's lifetime: it follows the directory's configuration through
// WatchResources, so its ID, grant and lifecycle token never change.
export interface ActiveLease extends Applied {
  readonly prepared: Prepared;
  readonly configuration: string;
  environmentInputs: Record<string, string>;
  origin: string;
  instance: string;
  renewAt: number;
}

const skillsOf = (skills: LeaseResources["content"]["skills"]) =>
  skills.map((skill) => `${skill.id}:${skill.digest}`).join("\n");

const urlsOf = (applied: Applied) =>
  applied.endpoints.map((endpoint) => `${endpoint.name}:${endpoint.url.href}`).join("\n");

export const acquireLease = Effect.fn("acquireLease")(function* (
  onChange: (changes: ResourceChanges) => Effect.Effect<void>,
) {
  const client = yield* AgxClient;
  const connection = yield* AgxConnection;
  const configuration = yield* Configuration;
  const deployer = yield* SkillDeployer;
  const crypto = yield* Crypto.Crypto;
  const lock = Semaphore.makeUnsafe(1);

  const environmentFor = Effect.fn("lease.environmentFor")(function* (
    names: ReadonlyArray<string>,
  ) {
    const inputs: Record<string, string> = {};
    for (const name of names) {
      const value = yield* Config.option(Config.String(name)).pipe(
        Effect.catchTag("ConfigError", (cause) =>
          Effect.fail(
            new IntegrationError({ operation: "resource environment", reason: "options", cause }),
          ),
        ),
      );
      if (Option.isSome(value)) {
        inputs[name] = value.value;
      }
    }
    return inputs;
  });

  const apply = Effect.fn("lease.apply")(function* (
    target: Discovery,
    resources: LeaseResources,
  ): Effect.fn.Return<Applied, IntegrationError> {
    if (resources.settings.length > 0) {
      return yield* new IntegrationError({
        operation: "native settings are unsupported",
        reason: "settings",
      });
    }
    if (resources.mcp.some((endpoint) => endpoint.url.origin !== target.url.origin)) {
      return yield* new IntegrationError({ operation: "MCP endpoint origin", reason: "content" });
    }
    const skills = yield* Effect.forEach(resources.content.skills, (tree) =>
      deployer.prepare(tree),
    );
    const endpoints = yield* Effect.forEach(
      resources.mcp,
      Effect.fn("lease.endpoint")(function* (endpoint) {
        const digest = yield* crypto
          .digest("SHA-256", new TextEncoder().encode(endpoint.id))
          .pipe(
            Effect.catchTag("PlatformError", (cause) =>
              Effect.fail(
                new IntegrationError({ operation: "MCP identity", reason: "content", cause }),
              ),
            ),
          );
        const label = endpoint.id.replace(/[^a-zA-Z0-9_-]/g, "-").slice(0, 15);
        return { name: `agx-${label}-${Hex.encode(digest).slice(0, 12)}`, url: endpoint.url };
      }),
    );
    return { resources, skills, endpoints };
  });

  const initial = yield* connection.discover();
  const resolve = yield* client.resolve(initial, {
    directory: configuration.scopeDirectory,
    harness: configuration.harness,
  });
  const snapshot = yield* Schema.decodeUnknownEffect(Snapshot)(resolve.snapshot).pipe(
    Effect.catchTag("SchemaError", (cause) =>
      Effect.fail(
        new IntegrationError({ operation: "resolved snapshot", reason: "content", cause }),
      ),
    ),
  );
  const environmentInputs = yield* environmentFor(snapshot.requiredEnvironment);
  const raw = yield* client.prepare(initial, {
    directory: configuration.scopeDirectory,
    harness: configuration.harness,
    environmentInputs,
    mcpDirectory: configuration.mcpDirectory,
  });
  const envelope = yield* Schema.decodeUnknownEffect(LeaseEnvelope)(raw).pipe(
    Effect.catchTag("SchemaError", (cause) =>
      Effect.fail(
        new IntegrationError({ operation: "resource lease credentials", reason: "content", cause }),
      ),
    ),
  );
  yield* Effect.addFinalizer(() =>
    connection.poll().pipe(
      Effect.flatMap((current) =>
        client.release(current, envelope.launch.id, envelope.lifecycleToken),
      ),
      Effect.catchTags({
        ConnectionError: (error) => Effect.logWarning(error.message),
        DaemonError: (error) => Effect.logWarning(error.message),
      }),
    ),
  );
  const prepared = yield* Schema.decodeUnknownEffect(Prepared)(raw).pipe(
    Effect.catchTag("SchemaError", (cause) =>
      Effect.fail(
        new IntegrationError({ operation: "prepared resources", reason: "content", cause }),
      ),
    ),
  );
  const active: ActiveLease = {
    ...(yield* apply(initial, prepared.resources)),
    prepared,
    configuration: initial.config,
    environmentInputs,
    origin: initial.url.origin,
    instance: initial.instance_id,
    renewAt: (yield* Clock.currentTimeMillis) + prepared.renewAfterSeconds * 1000,
  };

  // Changes a failed catalog reload left unapplied, retried with the next event.
  let pending: ResourceChanges = { skills: false, mcp: false };
  const receive = Effect.fn("lease.receive")(function* (
    target: Discovery,
    resources: LeaseResources,
  ) {
    const next = yield* apply(target, resources);
    const changes: ResourceChanges = {
      skills:
        pending.skills ||
        skillsOf(active.resources.content.skills) !== skillsOf(next.resources.content.skills),
      mcp:
        pending.mcp ||
        active.resources.mcpRevision !== next.resources.mcpRevision ||
        urlsOf(active) !== urlsOf(next),
    };
    Object.assign(active, next, { origin: target.url.origin });
    if (changes.skills || changes.mcp) {
      pending = changes;
      yield* onChange(changes);
      pending = { skills: false, mcp: false };
    }
  });

  const restore = Effect.fn("lease.restore")(function* (target: Discovery) {
    yield* client.attach(target, {
      launch: prepared.launch,
      grant: prepared.grant,
      lifecycleToken: prepared.lifecycleToken,
      environmentInputs: active.environmentInputs,
      mcpDirectory: prepared.mcpDirectory,
    });
    active.instance = target.instance_id;
    active.renewAt = (yield* Clock.currentTimeMillis) + prepared.renewAfterSeconds * 1000;
  });

  const synchronize = Effect.fn("lease.synchronize")(function* () {
    const target = yield* connection.poll();
    if (active.instance !== target.instance_id) {
      return yield* restore(target);
    }
    const now = yield* Clock.currentTimeMillis;
    if (now < active.renewAt) {
      return undefined;
    }
    yield* client
      .renew(target, prepared.launch.id, prepared.lifecycleToken)
      .pipe(
        Effect.catchTag("DaemonError", (error) =>
          error.code === Code.NotFound || error.ended === "expired"
            ? restore(target)
            : Effect.fail(error),
        ),
      );
    active.renewAt = now + prepared.renewAfterSeconds * 1000;
    return undefined;
  });

  let terminal: DaemonError | null = null;
  const refused = (error: DaemonError) =>
    error.ended === "revoked" || error.ended === "finished"
      ? Effect.sync(() => {
          terminal = error;
        }).pipe(Effect.andThen(Effect.fail(error)))
      : Effect.fail(error);
  const ensure: Effect.Effect<void, IntegrationFailure> = Effect.suspend(() =>
    terminal === null ? synchronize() : Effect.fail(terminal),
  ).pipe(
    Effect.catchTag("DaemonError", (error) =>
      error.code === Code.Unavailable ||
      error.code === Code.Unknown ||
      error.code === Code.DeadlineExceeded
        ? connection.reconnect(active.instance).pipe(Effect.andThen(synchronize()))
        : refused(error),
    ),
    lock.withPermits(1),
  );

  // The daemon reconciles the lease while this watch is open and sends what it provides when that
  // changes. A watch carries the inputs it opened with, so one naming an input this process holds
  // but has not sent ends and opens again with it.
  let invalid = false;
  const consume = Effect.fn("lease.consume")(function* (target: Discovery, message: unknown) {
    const event = yield* Schema.decodeUnknownEffect(ResourceWatchEvent)(message).pipe(
      Effect.catchTag("SchemaError", (cause) =>
        Effect.fail(
          new IntegrationError({ operation: "resource watch event", reason: "content", cause }),
        ),
      ),
    );
    if (event.state.case === "invalid") {
      if (!invalid) {
        invalid = true;
        yield* Effect.logWarning("AGX resources are invalid; keeping the last valid resources");
      }
      return true;
    }
    if (invalid) {
      invalid = false;
      yield* Effect.logInfo("AGX resources are valid again");
    }
    const resources = event.state.value;
    const wanted = yield* environmentFor(resources.snapshot.requiredEnvironment);
    const sent = active.environmentInputs;
    if (
      Object.keys(wanted).length !== Object.keys(sent).length ||
      Object.entries(wanted).some(([name, value]) => sent[name] !== value)
    ) {
      active.environmentInputs = wanted;
      return false;
    }
    yield* receive(target, resources);
    return true;
  }, lock.withPermits(1));
  const watch = Effect.gen(function* () {
    yield* ensure;
    const target = yield* connection.poll();
    yield* client
      .watch(target, prepared.launch.id, active.environmentInputs, prepared.lifecycleToken)
      .pipe(Stream.runForEachWhile((message) => consume(target, message)));
  }).pipe(Effect.catchTag("DaemonError", refused));
  yield* Effect.suspend(() => (terminal === null ? watch : Effect.never)).pipe(
    Effect.catchTags({
      ConnectionError: (error) => Effect.logWarning(error.message),
      DaemonError: (error) => Effect.logWarning(error.message),
      IntegrationError: (error) => Effect.logWarning(error.message),
    }),
    Effect.repeat(Schedule.spaced("1 second")),
    Effect.forkScoped,
  );

  return { current: (): ActiveLease => active, ensure };
});
