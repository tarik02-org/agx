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
import type * as Scope from "effect/Scope";
import * as ScopedRef from "effect/ScopedRef";
import * as Semaphore from "effect/Semaphore";

import { Configuration } from "./Configuration.ts";
import { IntegrationError, type IntegrationFailure } from "./error.ts";
import { LeaseEnvelope, Prepared, Snapshot } from "./schema.ts";
import { SkillDeployer } from "./SkillDeployer.ts";

export interface ActiveLease {
  readonly prepared: Prepared;
  readonly skills: ReadonlyArray<Skill.Info>;
  readonly environmentInputs: Record<string, string>;
  readonly configuration: string;
  readonly origin: string;
  readonly endpoints: ReadonlyArray<{ readonly name: string; readonly url: URL }>;
  instance: string;
  renewAt: number;
}

export const acquireLease = Effect.fn("acquireLease")(function* (onChange: Effect.Effect<void>) {
  const client = yield* AgxClient;
  const connection = yield* AgxConnection;
  const configuration = yield* Configuration;
  const deployer = yield* SkillDeployer;
  const crypto = yield* Crypto.Crypto;
  const lock = Semaphore.makeUnsafe(1);

  const acquire = Effect.fn("lease.acquire")(function* (): Effect.fn.Return<
    ActiveLease,
    IntegrationFailure,
    Scope.Scope
  > {
    const target = yield* connection.discover();
    const resolve = yield* client.resolve(target, {
      directory: configuration.scopeDirectory,
      harness: configuration.harness,
    });
    if (resolve.settings.length > 0) {
      return yield* new IntegrationError({
        operation: "native settings are unsupported",
        reason: "settings",
      });
    }
    const snapshot = yield* Schema.decodeUnknownEffect(Snapshot)(resolve.snapshot).pipe(
      Effect.catchTag("SchemaError", (cause) =>
        Effect.fail(
          new IntegrationError({ operation: "resolved snapshot", reason: "content", cause }),
        ),
      ),
    );
    const environmentInputs: Record<string, string> = {};
    for (const name of snapshot.requiredEnvironment) {
      const value = yield* Config.option(Config.String(name)).pipe(
        Effect.catchTag("ConfigError", (cause) =>
          Effect.fail(
            new IntegrationError({ operation: "resource environment", reason: "options", cause }),
          ),
        ),
      );
      if (Option.isSome(value)) {
        environmentInputs[name] = value.value;
      }
    }
    const raw = yield* client.prepare(target, {
      snapshotId: snapshot.id,
      directory: configuration.scopeDirectory,
      harness: configuration.harness,
      environmentInputs,
      mcpDirectory: configuration.mcpDirectory,
    });
    const envelope = yield* Schema.decodeUnknownEffect(LeaseEnvelope)(raw).pipe(
      Effect.catchTag("SchemaError", (cause) =>
        Effect.fail(
          new IntegrationError({
            operation: "resource lease credentials",
            reason: "content",
            cause,
          }),
        ),
      ),
    );
    yield* Effect.addFinalizer(() =>
      connection
        .poll()
        .pipe(
          Effect.flatMap((current) =>
            client.release(current, envelope.launch.id, envelope.lifecycleToken),
          ),
        )
        .pipe(
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
    if (prepared.mcp.some((endpoint) => endpoint.url.origin !== target.url.origin)) {
      return yield* new IntegrationError({ operation: "MCP endpoint origin", reason: "content" });
    }
    const skills = yield* Effect.forEach(prepared.content.skills, (tree) => deployer.prepare(tree));
    const endpoints = yield* Effect.forEach(prepared.mcp, (endpoint) =>
      Effect.gen(function* () {
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
    const now = yield* Clock.currentTimeMillis;
    return {
      prepared,
      skills,
      environmentInputs,
      configuration: target.config,
      origin: target.url.origin,
      endpoints,
      instance: target.instance_id,
      renewAt: now + prepared.renewAfterSeconds * 1000,
    };
  });

  const acquireCurrent = acquire().pipe(
    Effect.retry({
      times: 2,
      while: (error) => error._tag === "DaemonError" && error.code === Code.Aborted,
    }),
  );
  const state = yield* ScopedRef.fromAcquire(acquireCurrent);
  let terminal: DaemonError | null = null;
  const replace = Effect.fn("lease.replace")(function* () {
    yield* ScopedRef.set(state, acquireCurrent);
    yield* onChange;
    terminal = null;
  });
  const restore = Effect.fn("lease.restore")(function* (target: Discovery, active: ActiveLease) {
    yield* client.attach(target, {
      launch: active.prepared.launch,
      grant: active.prepared.grant,
      lifecycleToken: active.prepared.lifecycleToken,
      environmentInputs: active.environmentInputs,
      mcpDirectory: active.prepared.mcpDirectory,
    });
    const resolved = yield* client.resolve(target, {
      directory: configuration.scopeDirectory,
      harness: configuration.harness,
    });
    if (
      resolved.snapshot?.id !== active.prepared.launch.snapshotId ||
      active.origin !== target.url.origin
    ) {
      yield* replace();
      return;
    }
    yield* client.renew(target, active.prepared.launch.id, active.prepared.lifecycleToken);
    active.instance = target.instance_id;
    active.renewAt = (yield* Clock.currentTimeMillis) + active.prepared.renewAfterSeconds * 1000;
  });

  const synchronize = Effect.fn("lease.synchronize")(function* () {
    const target = yield* connection.poll();
    const active = yield* ScopedRef.get(state);
    if (active.instance !== target.instance_id) {
      yield* restore(target, active);
      return;
    }
    const now = yield* Clock.currentTimeMillis;
    if (now < active.renewAt) {
      return;
    }
    yield* client
      .renew(target, active.prepared.launch.id, active.prepared.lifecycleToken)
      .pipe(
        Effect.catchTag("DaemonError", (error) =>
          error.code === Code.NotFound || error.ended === "expired"
            ? restore(target, active)
            : Effect.fail(error),
        ),
      );
    active.renewAt = now + active.prepared.renewAfterSeconds * 1000;
  });

  const ensure = Effect.suspend(() =>
    terminal === null ? synchronize() : Effect.fail(terminal),
  ).pipe(
    Effect.catchTag("DaemonError", (error) =>
      error.ended === "revoked" || error.ended === "finished"
        ? Effect.sync(() => {
            terminal = error;
          }).pipe(Effect.andThen(Effect.fail(error)))
        : error.code === Code.Unavailable ||
            error.code === Code.Unknown ||
            error.code === Code.DeadlineExceeded
          ? connection
              .reconnect(ScopedRef.getUnsafe(state).instance)
              .pipe(Effect.andThen(synchronize()))
          : Effect.fail(error),
    ),
    lock.withPermits(1),
  );
  return {
    current: () => ScopedRef.getUnsafe(state),
    ensure,
    refresh: replace().pipe(lock.withPermits(1)),
  };
});
