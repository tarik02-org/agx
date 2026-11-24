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
import * as Schema from "effect/Schema";
import * as Semaphore from "effect/Semaphore";

import { Configuration } from "./Configuration.ts";
import { IntegrationError } from "./error.ts";
import { LeaseEnvelope, LeaseResources, Prepared, Snapshot } from "./schema.ts";
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
// SyncResources, so its ID, grant and lifecycle token never change.
export interface ActiveLease extends Applied {
  readonly prepared: Prepared;
  readonly configuration: string;
  environmentInputs: Record<string, string>;
  origin: string;
  instance: string;
  renewAt: number;
}

const decodeResources = (raw: unknown) =>
  Schema.decodeUnknownEffect(LeaseResources)(raw).pipe(
    Effect.catchTag("SchemaError", (cause) =>
      Effect.fail(new IntegrationError({ operation: "lease resources", reason: "content", cause })),
    ),
  );

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

  // Changes a failed catalog reload left unapplied, retried by the next sync.
  let pending: ResourceChanges = { skills: false, mcp: false };
  const sync = Effect.fn("lease.sync")(function* (target: Discovery) {
    const request = (inputs: Record<string, string>) =>
      client
        .sync(target, prepared.launch.id, inputs, prepared.lifecycleToken)
        .pipe(Effect.flatMap((response) => decodeResources(response.resources)));
    let inputs = active.environmentInputs;
    let resources = yield* request(inputs);
    // The daemon names the inputs its current selection reads; values only go to those names.
    const wanted = yield* environmentFor(resources.snapshot.requiredEnvironment);
    if (
      Object.keys(wanted).length !== Object.keys(inputs).length ||
      Object.entries(wanted).some(([name, value]) => inputs[name] !== value)
    ) {
      inputs = wanted;
      resources = yield* request(inputs);
    }
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
    Object.assign(active, next, { environmentInputs: inputs, origin: target.url.origin });
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
    yield* sync(target);
  });

  const synchronize = Effect.fn("lease.synchronize")(function* (refresh: boolean) {
    const target = yield* connection.poll();
    if (active.instance !== target.instance_id) {
      return yield* restore(target);
    }
    const now = yield* Clock.currentTimeMillis;
    if (refresh) {
      return yield* sync(target).pipe(
        Effect.catchTag("DaemonError", (error) =>
          error.code === Code.NotFound || error.ended === "expired"
            ? restore(target)
            : Effect.fail(error),
        ),
      );
    }
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
  const guarded = (refresh: boolean) =>
    Effect.suspend(() => (terminal === null ? synchronize(refresh) : Effect.fail(terminal))).pipe(
      Effect.catchTag("DaemonError", (error) =>
        error.ended === "revoked" || error.ended === "finished"
          ? Effect.sync(() => {
              terminal = error;
            }).pipe(Effect.andThen(Effect.fail(error)))
          : error.code === Code.Unavailable ||
              error.code === Code.Unknown ||
              error.code === Code.DeadlineExceeded
            ? connection.reconnect(active.instance).pipe(Effect.andThen(synchronize(refresh)))
            : Effect.fail(error),
      ),
      lock.withPermits(1),
    );
  return {
    current: (): ActiveLease => active,
    ensure: guarded(false),
    refresh: guarded(true),
  };
});
