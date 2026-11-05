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
import { SessionCommandFailure_Code, type SessionCommand } from "agx-client/protocol";
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
import { NativeRoute, NativeSessionRouting } from "./NativeSessionRouting.ts";

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
  sessionServer: Schema.optionalKey(
    Schema.URLFromString.check(
      Schema.makeFilter(
        (url) =>
          (url.protocol === "http:" || url.protocol === "https:") &&
          url.username === "" &&
          url.password === "" &&
          url.search === "" &&
          url.hash === "",
      ),
    ),
  ),
});
const LaunchData = Schema.Struct({
  sessionId: Schema.NonEmptyString,
  directory: Schema.NonEmptyString,
  ...NativeRoute.fields,
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
      "Session.NotFoundError": (cause) =>
        new NativeSessionError({ operation, reason: "not-found", cause }),
      SessionNotFoundError: (cause) =>
        new NativeSessionError({ operation, reason: "not-found", cause }),
      LocationNotFoundError: (cause) =>
        new NativeSessionError({ operation, reason: "not-found", cause }),
      ServiceUnavailableError: (cause) =>
        new NativeSessionError({ operation, reason: "unavailable", cause }),
      UnauthorizedError: (cause) =>
        new NativeSessionError({ operation, reason: "rejected", cause }),
      InvalidRequestError: (cause) =>
        new NativeSessionError({ operation, reason: "rejected", cause }),
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
  const routing = yield* NativeSessionRouting;
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
      NativeSessionError: (error) => Effect.logWarning(error.message),
      SchemaError: () => Effect.logWarning("OpenCode session catalog returned invalid data"),
      DaemonError: (error) => Effect.logWarning(error.message),
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
    const command = Effect.fn("sessions.command")(function* (request: SessionCommand) {
      return yield* Effect.gen(function* () {
        const sessionID = yield* Schema.decodeUnknownEffect(Session.ID)(request.nativeSessionId);
        const response = yield* Effect.result(ctx.session.get({ sessionID }));
        if (Result.isFailure(response)) {
          return yield* failNative("session validation", response.failure);
        }
        const session = response.success;
        const validated = yield* Schema.decodeUnknownEffect(Schema.toType(Session.Info))(session);
        if (
          validated.location.directory !== ctx.location.directory ||
          validated.location.workspaceID !== ctx.location.workspaceID ||
          validated.parentID !== undefined ||
          validated.time.archived !== undefined
        ) {
          return yield* client.reply(target, {
            source,
            commandId: request.id,
            result: {
              case: "failure",
              value: {
                code: SessionCommandFailure_Code.REJECTED,
                message: "Session is outside this location or is not listable",
              },
            },
          });
        }
        const launchDataJson = yield* Schema.encodeEffect(Schema.fromJsonString(LaunchData))({
          sessionId: validated.id,
          directory: validated.location.directory,
          ...(yield* routing.route(options.sessionServer ?? null)),
        });
        return yield* client.reply(target, {
          source,
          commandId: request.id,
          result: { case: "prepared", value: { session: summary(validated), launchDataJson } },
        });
      }).pipe(
        Effect.catchTags({
          NativeRoutingError: (error) =>
            Effect.logWarning(error.message).pipe(
              Effect.andThen(
                client.reply(target, {
                  source,
                  commandId: request.id,
                  result: {
                    case: "failure",
                    value: {
                      code: SessionCommandFailure_Code.REJECTED,
                      message: "Native resume routing is unavailable",
                    },
                  },
                }),
              ),
            ),
          NativeSessionError: (error) =>
            client.reply(target, {
              source,
              commandId: request.id,
              result: {
                case: "failure",
                value: {
                  code:
                    error.reason === "not-found"
                      ? SessionCommandFailure_Code.NOT_FOUND
                      : error.reason === "unavailable"
                        ? SessionCommandFailure_Code.UNAVAILABLE
                        : SessionCommandFailure_Code.REJECTED,
                  message: "Native session could not be validated",
                },
              },
            }),
          SchemaError: () =>
            client.reply(target, {
              source,
              commandId: request.id,
              result: {
                case: "failure",
                value: {
                  code: SessionCommandFailure_Code.REJECTED,
                  message: "Invalid native session",
                },
              },
            }),
        }),
      );
    });
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
            NativeSessionError: (error) => Effect.logWarning(error.message),
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
        yield* client.watch(target, { source }).pipe(
          Stream.runForEach((event) => {
            if (event.event.case === "ready") {
              return Deferred.succeed(ready, undefined).pipe(
                Effect.andThen(publish(target, source)),
                Effect.andThen(Deferred.succeed(initialized, undefined)),
              );
            }
            if (event.event.case === "command") {
              return command(event.event.value);
            }
            return Effect.void;
          }),
        );
      }),
    ).pipe(
      Effect.catchTag("DaemonError", (error) =>
        error.code === Code.FailedPrecondition
          ? Effect.never
          : connection.reconnect(target.instance_id).pipe(Effect.andThen(Effect.fail(error))),
      ),
    );
  });
  yield* serve().pipe(
    Effect.catchTags({
      ConnectionError: (error) => Effect.logWarning(error.message),
      DaemonError: (error) => Effect.logWarning(error.message),
      NativeSessionError: (error) => Effect.logWarning(error.message),
      SchemaError: () => Effect.logWarning("OpenCode session catalog returned invalid data"),
    }),
    Effect.repeat(Schedule.spaced("1 second")),
    Effect.forkScoped,
  );
  yield* Deferred.await(initialized).pipe(Effect.timeoutOption("5 seconds"));
});
