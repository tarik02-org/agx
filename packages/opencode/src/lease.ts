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
import * as Exit from "effect/Exit";
import * as Option from "effect/Option";
import * as Schedule from "effect/Schedule";
import * as Schema from "effect/Schema";
import * as Scope from "effect/Scope";
import * as Semaphore from "effect/Semaphore";
import * as Stream from "effect/Stream";

import { Configuration } from "./Configuration.ts";
import { IntegrationError, type IntegrationFailure } from "./error.ts";
import { LeaseEnvelope, Manifest, Prepared, ResourceWatchEvent } from "./schema.ts";
import { SkillDeployer } from "./SkillDeployer.ts";

export interface ResourceChanges {
  readonly skills: boolean;
  readonly mcp: boolean;
}

export interface ActiveLease {
  readonly prepared: Prepared;
  readonly skills: ReadonlyArray<Skill.Info>;
  readonly environmentInputs: Record<string, string>;
  readonly configuration: string;
  readonly origin: string;
  readonly endpoints: ReadonlyArray<{ readonly name: string; readonly url: URL }>;
  readonly scope: Scope.Closeable;
  // An unpublished successor retires itself back to its predecessor on acquisition failure.
  // After publication, its finalizer intentionally releases the authorization family.
  retireTo: string | null;
  instance: string;
  renewAt: number;
}

export const acquireLease = Effect.fn("acquireLease")(function* (
  onChange: (changes: ResourceChanges) => Effect.Effect<void>,
) {
  const client = yield* AgxClient;
  const connection = yield* AgxConnection;
  const configuration = yield* Configuration;
  const deployer = yield* SkillDeployer;
  const crypto = yield* Crypto.Crypto;
  const owner = yield* Effect.scope;
  const lock = Semaphore.makeUnsafe(1);
  let terminal: DaemonError | null = null;

  const refused = Effect.fn("lease.refused")(function* (error: DaemonError) {
    if (error.ended === "revoked" || error.ended === "finished") {
      terminal = error;
    }
    return yield* Effect.fail(error);
  });

  const acquire = Effect.fn("lease.acquire")(function* (
    target: Discovery,
    scope: Scope.Closeable,
    previous: ActiveLease | null,
  ): Effect.fn.Return<ActiveLease, IntegrationFailure, Scope.Scope> {
    const resolve = yield* client.resolve(target, {
      directory: configuration.scopeDirectory,
      harness: configuration.harness,
    });
    const manifest = yield* Schema.decodeUnknownEffect(Manifest)(resolve.manifest).pipe(
      Effect.catchTag("SchemaError", (cause) =>
        Effect.fail(
          new IntegrationError({ operation: "resource manifest", reason: "content", cause }),
        ),
      ),
    );
    if (manifest.settings.length > 0) {
      return yield* new IntegrationError({
        operation: "native settings are unsupported",
        reason: "settings",
      });
    }
    const environmentInputs: Record<string, string> = {};
    for (const name of manifest.snapshot.requiredEnvironment) {
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
    const input = {
      snapshotId: manifest.revision,
      environmentInputs,
      mcpDirectory: configuration.mcpDirectory,
    };
    let acquired: ActiveLease | null = null;
    const response = yield* Effect.acquireRelease(
      Effect.gen(function* () {
        const raw = yield* previous === null
          ? client.prepare(target, {
              ...input,
              directory: configuration.scopeDirectory,
              harness: configuration.harness,
            })
          : client
              .refresh(
                target,
                { ...input, launchId: previous.prepared.launch.id },
                previous.prepared.lifecycleToken,
              )
              .pipe(Effect.map((refreshed) => refreshed.resources));
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
        return { raw, envelope };
      }),
      ({ envelope }) =>
        connection.poll().pipe(
          Effect.flatMap((current) => {
            const retireTo =
              acquired === null ? (previous?.prepared.launch.id ?? null) : acquired.retireTo;
            return retireTo === null
              ? client.release(current, envelope.launch.id, envelope.lifecycleToken)
              : client.retire(current, envelope.launch.id, retireTo, envelope.lifecycleToken);
          }),
          Effect.catchTags({
            ConnectionError: (error) => Effect.logWarning(error.message),
            DaemonError: (error) => Effect.logWarning(error.message),
          }),
        ),
    );
    const prepared = yield* Schema.decodeUnknownEffect(Prepared)(response.raw).pipe(
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
    const endpoints = yield* Effect.forEach(
      prepared.mcp,
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
    acquired = {
      prepared,
      skills,
      environmentInputs,
      configuration: target.config,
      origin: target.url.origin,
      endpoints,
      scope,
      retireTo: previous?.prepared.launch.id ?? null,
      instance: target.instance_id,
      renewAt: (yield* Clock.currentTimeMillis) + prepared.renewAfterSeconds * 1000,
    };
    return acquired;
  });

  const acquireGeneration = Effect.fn("lease.acquireGeneration")(function* (
    target: Discovery,
    previous: ActiveLease | null,
    scope: Scope.Closeable,
  ) {
    return yield* acquire(target, scope, previous).pipe(
      Scope.provide(scope),
      Effect.retry({
        times: 2,
        while: (error) => error._tag === "DaemonError" && error.code === Code.Aborted,
      }),
      Effect.onExit((exit) => (Exit.isFailure(exit) ? Scope.close(scope, exit) : Effect.void)),
    );
  });

  let active = yield* acquireGeneration(
    yield* connection.discover(),
    null,
    Scope.forkUnsafe(owner),
  );
  active.retireTo = null;

  const maintain = Effect.fn("lease.maintain")(function* (
    target: Discovery,
    leases: ReadonlyArray<ActiveLease>,
  ) {
    yield* Effect.forEach(leases, (lease) =>
      client.renew(target, lease.prepared.launch.id, lease.prepared.lifecycleToken),
    );
    yield* Effect.sleep(
      Math.min(...leases.map((lease) => lease.prepared.renewAfterSeconds)) * 1000,
    );
  }, Effect.forever());

  const replace = Effect.fn("lease.replace")(
    (target: Discovery) =>
      Effect.uninterruptibleMask((restore) =>
        Effect.gen(function* () {
          if (terminal !== null) {
            return yield* Effect.fail(terminal);
          }
          const previous = active;
          const scope = Scope.forkUnsafe(owner);
          // Catalog reloads can exceed the lease TTL while the ordinary ensure loop waits
          // for this lock. Keep both generations live, and let revocation abort the handover.
          const next = yield* restore(
            Effect.raceFirst(
              acquireGeneration(target, previous, scope),
              maintain(target, [previous]),
            ).pipe(
              Effect.onExit((exit) =>
                Exit.isFailure(exit) ? Scope.close(scope, exit) : Effect.void,
              ),
            ),
          );
          const beforeSkills = previous.prepared.manifest.resources.filter(
            (entry) => entry.resource.resource.case === "skillId",
          );
          const afterSkills = next.prepared.manifest.resources.filter(
            (entry) => entry.resource.resource.case === "skillId",
          );
          const changes: ResourceChanges = {
            skills:
              beforeSkills.length !== afterSkills.length ||
              beforeSkills.some(
                (entry, index) =>
                  entry.digest !== afterSkills[index]?.digest ||
                  entry.resource.resource.value !== afterSkills[index]?.resource.resource.value,
              ),
            mcp: previous.endpoints.length > 0 || next.endpoints.length > 0,
          };
          active = next;
          const applied = yield* Effect.exit(
            restore(Effect.raceFirst(onChange(changes), maintain(target, [previous, next]))),
          );
          if (Exit.isFailure(applied)) {
            active = previous;
            yield* Effect.raceFirst(onChange(changes), maintain(target, [previous])).pipe(
              Effect.ensuring(Scope.close(next.scope, applied)),
            );
            return yield* Effect.failCause(applied.cause);
          }
          next.retireTo = null;
          previous.retireTo = next.prepared.launch.id;
          return yield* Scope.close(previous.scope, Exit.void);
        }),
      ),
    Effect.catchTag("DaemonError", refused),
  );

  const restore = Effect.fn("lease.restore")(function* (target: Discovery) {
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
      resolved.snapshot?.id !== active.prepared.manifest.revision ||
      active.origin !== target.url.origin
    ) {
      yield* replace(target);
      return;
    }
    yield* client.renew(target, active.prepared.launch.id, active.prepared.lifecycleToken);
    active.instance = target.instance_id;
    active.renewAt = (yield* Clock.currentTimeMillis) + active.prepared.renewAfterSeconds * 1000;
  });

  const synchronize = Effect.fn("lease.synchronize")(function* () {
    if (terminal !== null) {
      return yield* Effect.fail(terminal);
    }
    const target = yield* connection.poll();
    if (active.instance !== target.instance_id) {
      return yield* restore(target);
    }
    const now = yield* Clock.currentTimeMillis;
    if (now < active.renewAt) {
      return undefined;
    }
    yield* client
      .renew(target, active.prepared.launch.id, active.prepared.lifecycleToken)
      .pipe(
        Effect.catchTag("DaemonError", (error) =>
          error.code === Code.NotFound || error.ended === "expired"
            ? restore(target)
            : Effect.fail(error),
        ),
      );
    active.renewAt = now + active.prepared.renewAfterSeconds * 1000;
    return undefined;
  });

  const ensure = synchronize().pipe(
    Effect.catchTag("DaemonError", (error) =>
      error.code === Code.Unavailable ||
      error.code === Code.Unknown ||
      error.code === Code.DeadlineExceeded
        ? connection.reconnect(active.instance).pipe(Effect.andThen(synchronize()))
        : refused(error),
    ),
    Effect.catchTag("DaemonError", refused),
    lock.withPermits(1),
  );

  let invalid = false;
  const consume = Effect.fn("lease.consume")(
    function* (raw: unknown) {
      if (terminal !== null) {
        return;
      }
      const event = yield* Schema.decodeUnknownEffect(ResourceWatchEvent)(raw).pipe(
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
        return;
      }
      if (invalid) {
        invalid = false;
        yield* Effect.logInfo("AGX resources are valid again");
      }
      const previous = active;
      const instance = active.instance;
      yield* synchronize();
      if (
        active === previous &&
        active.instance === instance &&
        event.state.value.revision !== active.prepared.manifest.revision
      ) {
        yield* replace(yield* connection.poll());
      }
    },
    lock.withPermits(1),
    Effect.catchTag("DaemonError", refused),
  );

  const watch = Stream.unwrap(
    connection.poll().pipe(
      Effect.map((target) =>
        client.watch(target, {
          directory: configuration.scopeDirectory,
          harness: configuration.harness,
        }),
      ),
    ),
  ).pipe(Stream.runForEach(consume));
  yield* Effect.suspend(() => (terminal === null ? watch : Effect.never)).pipe(
    Effect.catchTags({
      ConnectionError: (error) => Effect.logWarning(error.message),
      IntegrationError: (error) => Effect.logWarning(error.message),
      DaemonError: (error) => Effect.logWarning(error.message),
    }),
    Effect.repeat(Schedule.spaced("1 second")),
    Effect.forkScoped,
  );

  const refresh = Effect.fn("lease.refresh")(
    function* () {
      const previous = active;
      yield* synchronize();
      if (active === previous) {
        yield* replace(yield* connection.poll());
      }
    },
    lock.withPermits(1),
    Effect.catchTag("DaemonError", refused),
  );

  return {
    current: () => active,
    ensure,
    refresh: refresh(),
  };
});
