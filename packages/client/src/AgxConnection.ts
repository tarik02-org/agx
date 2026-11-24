import * as Context from "effect/Context";
import * as Effect from "effect/Effect";
import * as FileSystem from "effect/FileSystem";
import * as Layer from "effect/Layer";
import * as ChildProcess from "effect/process/ChildProcess";
import * as ChildProcessSpawner from "effect/process/ChildProcessSpawner";
import * as Schema from "effect/Schema";

import { ConnectionError } from "./error.ts";
import { Bootstrap, BootstrapFailure, ConnectionOptions, Discovery } from "./schema.ts";

export class AgxConnection extends Context.Service<
  AgxConnection,
  {
    readonly discover: () => Effect.Effect<Discovery, ConnectionError>;
    readonly poll: () => Effect.Effect<Discovery, ConnectionError>;
    readonly reconnect: (instance: string) => Effect.Effect<Discovery, ConnectionError>;
  }
>()("agx-client/AgxConnection") {}

export const makeAgxConnection = Effect.fn("makeAgxConnection")(function* (
  input: ConnectionOptions = {},
) {
  const options = yield* Schema.decodeUnknownEffect(ConnectionOptions)(input).pipe(
    Effect.catchTag("SchemaError", (cause) =>
      Effect.fail(
        new ConnectionError({ operation: "connection options", reason: "options", cause }),
      ),
    ),
  );
  const fs = yield* FileSystem.FileSystem;
  const spawner = yield* ChildProcessSpawner.ChildProcessSpawner;
  const executable = options.executable ?? "agx";
  const explicitDiscovery = options.discovery ?? null;
  let discoveryFile = explicitDiscovery;

  const bootstrap = Effect.fn("AgxConnection.bootstrap")(function* (
    instance: string | null = null,
  ) {
    const args = ["--json"];
    if (options.config !== undefined) {
      args.push("--config", options.config);
    }
    args.push("daemon", "connect");
    if (instance !== null) {
      args.push("--recover-instance", instance);
    }
    const output = yield* spawner
      .string(ChildProcess.make(executable, args, { stderr: "ignore" }))
      .pipe(
        Effect.catchTag("PlatformError", (cause) =>
          Effect.fail(
            new ConnectionError({ operation: "daemon bootstrap", reason: "bootstrap", cause }),
          ),
        ),
      );
    const result = yield* Schema.decodeUnknownEffect(Schema.fromJsonString(Bootstrap))(output).pipe(
      Effect.catchTag("SchemaError", (cause) =>
        // A failed command prints why as its JSON document, such as a daemon that cannot start.
        Schema.decodeUnknownEffect(Schema.fromJsonString(BootstrapFailure))(output).pipe(
          Effect.matchEffect({
            onFailure: () =>
              Effect.fail(
                new ConnectionError({
                  operation: "daemon bootstrap response",
                  reason: "bootstrap",
                  cause,
                }),
              ),
            onSuccess: (failure) =>
              Effect.fail(
                new ConnectionError({
                  operation: "daemon bootstrap",
                  reason: "bootstrap",
                  detail: failure.message,
                }),
              ),
          }),
        ),
      ),
    );
    discoveryFile = result.data.discovery;
    return result.data.discovery;
  });

  const readDiscovery = Effect.fn("AgxConnection.readDiscovery")(function* (allowStartup: boolean) {
    if (discoveryFile === null && !allowStartup) {
      return yield* new ConnectionError({ operation: "daemon discovery", reason: "discovery" });
    }
    const path = discoveryFile === null ? yield* bootstrap() : discoveryFile;
    const text = yield* fs.readFileString(path).pipe(
      Effect.catchReason("PlatformError", "NotFound", (_reason, cause) =>
        explicitDiscovery === null && allowStartup
          ? bootstrap().pipe(Effect.flatMap((next) => fs.readFileString(next)))
          : Effect.fail(
              new ConnectionError({ operation: "daemon discovery", reason: "discovery", cause }),
            ),
      ),
      Effect.catchTag("PlatformError", (cause) =>
        Effect.fail(
          new ConnectionError({ operation: "daemon discovery", reason: "discovery", cause }),
        ),
      ),
    );
    return yield* Schema.decodeUnknownEffect(Schema.fromJsonString(Discovery))(text).pipe(
      Effect.catchTag("SchemaError", (cause) =>
        Effect.fail(
          new ConnectionError({
            operation: "daemon discovery document",
            reason: "discovery",
            cause,
          }),
        ),
      ),
    );
  });

  const discover = Effect.fn("AgxConnection.discover")(() => readDiscovery(true));
  const poll = Effect.fn("AgxConnection.poll")(() => readDiscovery(false));
  const reconnect = Effect.fn("AgxConnection.reconnect")((instance: string) =>
    explicitDiscovery === null
      ? bootstrap(instance).pipe(Effect.andThen(readDiscovery(false)))
      : readDiscovery(false),
  );

  return AgxConnection.of({ discover, poll, reconnect });
});

export const AgxConnectionLive = (options: ConnectionOptions = {}) =>
  Layer.effect(AgxConnection, makeAgxConnection(options));
