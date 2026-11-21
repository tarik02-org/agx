import { Code } from "@connectrpc/connect";
import type { Context } from "@opencode/plugin/effect/plugin";
import {
  InvalidRequestError,
  LocationNotFoundError,
  ServiceUnavailableError,
  SessionNotFoundError,
  UnauthorizedError,
} from "@opencode/protocol/errors";
import { Session } from "@opencode/schema/session";
import { AgxConnection } from "agx-client/AgxConnection";
import type { Discovery } from "agx-client/schema";
import {
  SessionProviderClient,
  type ProviderSessionInput,
  type SessionSourceInput,
} from "agx-client/SessionProviderClient";
import * as Crypto from "effect/Crypto";
import * as DateTime from "effect/DateTime";
import * as Deferred from "effect/Deferred";
import * as Effect from "effect/Effect";
import * as Hex from "effect/encoding/Hex";
import * as Match from "effect/Match";
import * as Option from "effect/Option";
import * as Result from "effect/Result";
import * as Schedule from "effect/Schedule";
import * as Schema from "effect/Schema";
import * as Semaphore from "effect/Semaphore";
import * as Stream from "effect/Stream";

import { Configuration } from "./Configuration.ts";

class NativeSessionError extends Schema.TaggedError<NativeSessionError>()("NativeSessionError", {
  operation: Schema.String,
  reason: Schema.Literals(["not-found", "unavailable", "rejected", "invalid"]),
  cause: Schema.Defect(),
}) {
  override get message() {
    return `OpenCode ${this.operation} failed (${this.reason})`;
  }
}

const sourceIdentityGate = Semaphore.makeUnsafe(1);

const SessionOptions = Schema.Struct({
  sessionSourceId: Schema.optionalKey(Schema.NonEmptyString.check(Schema.isMaxLength(256))),
});

const NativeFailure = Schema.Union([
  // In-process plugin calls currently expose this internal tag instead of the protocol error.
  Schema.Struct({ _tag: Schema.Literal("Session.NotFoundError"), sessionID: Session.ID }),
  SessionNotFoundError,
  LocationNotFoundError,
  ServiceUnavailableError,
  UnauthorizedError,
  InvalidRequestError,
]);

// Native methods expose unknown failures; classify their public error schemas at this adapter.
const failNative = Effect.fn("sessions.failNative")(function* (
  operation: string,
  failure: unknown,
): Effect.fn.Return<never, NativeSessionError> {
  const parsed = Schema.decodeUnknownOption(NativeFailure)(failure);
  if (Option.isNone(parsed)) {
    return yield* Effect.die(failure);
  }
  return yield* Match.value(parsed.value).pipe(
    Match.tagsExhaustive({
      "Session.NotFoundError": () =>
        new NativeSessionError({ operation, reason: "not-found", cause: failure }),
      SessionNotFoundError: () =>
        new NativeSessionError({ operation, reason: "not-found", cause: failure }),
      LocationNotFoundError: () =>
        new NativeSessionError({ operation, reason: "not-found", cause: failure }),
      ServiceUnavailableError: () =>
        new NativeSessionError({ operation, reason: "unavailable", cause: failure }),
      UnauthorizedError: () =>
        new NativeSessionError({ operation, reason: "rejected", cause: failure }),
      InvalidRequestError: () =>
        new NativeSessionError({ operation, reason: "rejected", cause: failure }),
    }),
  );
});

const summary = (session: Session.Info): ProviderSessionInput => ({
  id: session.id,
  directory: session.location.directory,
  title: session.title ?? "",
  message: "",
  startedAt: {
    seconds: BigInt(Math.floor(DateTime.toEpochMillis(session.time.created) / 1000)),
    nanos: (DateTime.toEpochMillis(session.time.created) % 1000) * 1_000_000,
  },
  updatedAt: {
    seconds: BigInt(Math.floor(DateTime.toEpochMillis(session.time.updated) / 1000)),
    nanos: (DateTime.toEpochMillis(session.time.updated) % 1000) * 1_000_000,
  },
});

export const registerSessions = Effect.fn("registerSessions")(function* (ctx: Context) {
  const configuration = yield* Configuration;
  const connection = yield* AgxConnection;
  const client = yield* SessionProviderClient;
  const crypto = yield* Crypto.Crypto;
  const options = yield* Schema.decodeUnknownEffect(SessionOptions)(ctx.options);
  const sourceId = yield* Effect.gen(function* () {
    if (options.sessionSourceId !== undefined) {
      return options.sessionSourceId;
    }
    // Native plugin storage survives service restarts and belongs to this native profile.
    const storedSource = yield* ctx.storage.get("agx.session-source-id");
    if (storedSource !== undefined) {
      return yield* Schema.decodeUnknownEffect(Schema.NonEmptyString)(storedSource);
    }
    const created = yield* crypto.randomUUIDv4();
    yield* ctx.storage.set("agx.session-source-id", created);
    return created;
  }).pipe(sourceIdentityGate.withPermits(1));
  const epoch = yield* crypto.randomUUIDv4();
  const scopeId = Hex.encode(
    yield* crypto.digest(
      "SHA-256",
      new TextEncoder().encode(`${ctx.location.directory}\0${ctx.location.workspaceID ?? ""}`),
    ),
  );
  const gate = yield* Semaphore.make(1);
  const initialized = yield* Deferred.make<void>();
  const list = Effect.fn("sessions.list")(function* () {
    const sessions: Array<Session.Info> = [];
    let cursor: Session.ListCursor | undefined;
    const visited = new Set<string>();
    do {
      const response = yield* Effect.result(
        ctx.session.list({
          directory: ctx.location.directory,
          parentID: null,
          archived: false,
          limit: 200,
          ...(cursor === undefined ? {} : { cursor }),
        }),
      );
      if (Result.isFailure(response)) {
        return yield* failNative("session list", response.failure);
      }
      const page = response.success;
      for (const value of page.data) {
        const session = yield* Schema.decodeUnknownEffect(Schema.toType(Session.Info))(value);
        if (
          session.location.directory === ctx.location.directory &&
          session.location.workspaceID === ctx.location.workspaceID &&
          session.parentID === undefined &&
          session.time.archived === undefined
        ) {
          sessions.push(session);
        }
      }
      cursor = page.cursor.next;
      if (cursor !== undefined) {
        if (visited.has(cursor)) {
          return yield* new NativeSessionError({
            operation: "session pagination",
            reason: "invalid",
            cause: "repeated cursor",
          });
        }
        visited.add(cursor);
      }
    } while (cursor !== undefined);
    return sessions;
  });
  const publish = Effect.fn("sessions.publish")(function* (
    target: Discovery,
    source: SessionSourceInput,
  ) {
    const sessions = yield* list();
    yield* client.publish(target, { source, replace: true, sessions: sessions.map(summary) });
  }, gate.withPermits(1));

  const reconcile = Effect.fn("sessions.reconcile")(
    (target: Discovery, source: SessionSourceInput) => publish(target, source),
    Effect.catchTags({
      NativeSessionError: (error) => Effect.logWarning(error),
      SchemaError: (cause) =>
        Effect.logWarning("OpenCode session catalog returned invalid data", cause),
      DaemonError: (error) => Effect.logWarning(error),
    }),
  );

  const serve = Effect.fn("sessions.serve")(function* () {
    const target = yield* connection.poll();
    const source = {
      sourceId,
      scopeId,
      epoch,
      builtin: "opencode",
      harness: configuration.harness,
    };
    const ready = yield* Deferred.make<void>();
    yield* Effect.scoped(
      Effect.gen(function* () {
        yield* Deferred.await(ready).pipe(
          Effect.andThen(
            ctx.event
              .subscribe({
                types: [
                  "session.created",
                  "session.renamed",
                  "session.archived",
                  "session.unarchived",
                  "session.deleted",
                  "session.moved",
                  "session.usage.updated",
                ],
              })
              .pipe(
                Stream.result,
                Stream.debounce("300 millis"),
                Stream.runForEach((result) =>
                  Result.isFailure(result)
                    ? failNative("session events", result.failure)
                    : reconcile(target, source),
                ),
              ),
          ),
          Effect.catchTags({
            NativeSessionError: (error) => Effect.logWarning(error),
          }),
          Effect.forkScoped,
        );
        yield* Deferred.await(ready).pipe(
          Effect.andThen(
            Effect.sleep("10 seconds").pipe(
              Effect.andThen(reconcile(target, source)),
              Effect.forever,
            ),
          ),
          Effect.forkScoped,
        );
        yield* client.connect(target, { source }).pipe(
          Stream.runForEach(() =>
            Deferred.succeed(ready, undefined).pipe(
              Effect.tap(() =>
                Effect.sync(() => {
                  reported = "";
                }),
              ),
              Effect.andThen(publish(target, source)),
              Effect.andThen(Deferred.succeed(initialized, undefined)),
            ),
          ),
        );
      }),
    ).pipe(
      Effect.catchTag("DaemonError", (error) =>
        error.code === Code.FailedPrecondition &&
        error.cause.metadata.get("Agx-Session-Source-Replaced") === "1"
          ? Effect.never
          : // Only a daemon that is gone is recovered: one that refuses the source, for its
            // configuration or its version, would refuse it again.
            error.code === Code.Unavailable ||
              error.code === Code.Unknown ||
              error.code === Code.DeadlineExceeded
            ? connection.reconnect(target.instance_id).pipe(Effect.andThen(Effect.fail(error)))
            : Effect.fail(error),
      ),
    );
  });
  // A failure that lasts, such as a stopped daemon, is reported once, not at every attempt, and
  // ends the wait for the first catalog: resources do not depend on it.
  let reported = "";
  const failed = (message: string, cause: unknown) =>
    Deferred.succeed(initialized, undefined).pipe(
      Effect.andThen(
        Effect.suspend(() => {
          if (message === reported) {
            return Effect.void;
          }
          reported = message;
          return Effect.logWarning(message, cause);
        }),
      ),
    );
  yield* serve().pipe(
    Effect.catchTags({
      ConnectionError: (error) => failed(error.message, error),
      DaemonError: (error) =>
        failed(error.message, error).pipe(
          Effect.andThen(
            error.code === Code.Unavailable ||
              error.code === Code.Unknown ||
              error.code === Code.DeadlineExceeded
              ? Effect.void
              : Effect.sleep("30 seconds"),
          ),
        ),
      NativeSessionError: (error) => failed(error.message, error),
      SchemaError: (cause) => failed("OpenCode session catalog returned invalid data", cause),
    }),
    Effect.repeat(Schedule.spaced("1 second")),
    Effect.forkScoped,
  );
  yield* Deferred.await(initialized).pipe(Effect.timeoutOption("5 seconds"));
});
