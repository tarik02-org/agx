import { Code, ConnectError, createClient, type CallOptions } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import * as Context from "effect/Context";
import * as Effect from "effect/Effect";
import * as FileSystem from "effect/FileSystem";
import * as Layer from "effect/Layer";
import * as Schema from "effect/Schema";
import * as ChildProcess from "effect/process/ChildProcess";
import * as ChildProcessSpawner from "effect/process/ChildProcessSpawner";

import {
  LaunchService,
  ResourceService,
  type AttachLaunchRequest,
  type Launch,
  type PrepareResourcesRequest,
  type PrepareResourcesResponse,
  type ResolveResourcesResponse,
} from "../../../gen/typescript/agx/v1/agx_pb.ts";
import { Configuration } from "./Configuration.ts";
import { DaemonError, EndState, IntegrationError, type IntegrationFailure } from "./error.ts";
import { Bootstrap, Discovery, type LifecycleToken } from "./schema.ts";

export class AgxClient extends Context.Service<
  AgxClient,
  {
    readonly discover: Effect.Effect<Discovery, IntegrationError>;
    readonly poll: Effect.Effect<Discovery, IntegrationError>;
    readonly reconnect: (instance: string) => Effect.Effect<Discovery, IntegrationError>;
    readonly resolve: (target: Discovery) => Effect.Effect<ResolveResourcesResponse, DaemonError>;
    readonly prepare: (
      target: Discovery,
      input: PrepareResourcesRequest,
    ) => Effect.Effect<PrepareResourcesResponse, DaemonError>;
    readonly renew: (
      target: Discovery,
      launch: Launch,
      token: LifecycleToken,
    ) => Effect.Effect<void, DaemonError>;
    readonly attach: (
      target: Discovery,
      input: AttachLaunchRequest,
    ) => Effect.Effect<void, DaemonError>;
    readonly release: (
      launch: Launch,
      token: LifecycleToken,
    ) => Effect.Effect<void, IntegrationFailure>;
  }
>()("agx-opencode/AgxClient") {}

export const makeAgxClient = Effect.fn("makeAgxClient")(function* () {
  const configuration = yield* Configuration;
  const fs = yield* FileSystem.FileSystem;
  const spawner = yield* ChildProcessSpawner.ChildProcessSpawner;
  let discoveryFile = configuration.discovery;

  const bootstrap = Effect.fn("AgxClient.bootstrap")(function* (instance: string | null = null) {
    const args = ["--json"];
    if (configuration.config !== null) args.push("--config", configuration.config);
    args.push("daemon", "connect");
    if (instance !== null) args.push("--recover-instance", instance);
    const output = yield* spawner
      .string(ChildProcess.make(configuration.executable, args, { stderr: "ignore" }))
      .pipe(
        Effect.mapError(
          (cause) =>
            new IntegrationError({ operation: "daemon bootstrap", reason: "bootstrap", cause }),
        ),
      );
    const result = yield* Schema.decodeUnknownEffect(Schema.fromJsonString(Bootstrap))(output).pipe(
      Effect.mapError(
        (cause) =>
          new IntegrationError({
            operation: "daemon bootstrap response",
            reason: "bootstrap",
            cause,
          }),
      ),
    );
    discoveryFile = result.data.discovery;
    return result.data.discovery;
  });

  const discover = Effect.fn("AgxClient.discover")(function* (allowStartup = true) {
    if (discoveryFile === null && !allowStartup) {
      return yield* new IntegrationError({ operation: "daemon discovery", reason: "discovery" });
    }
    const path = discoveryFile === null ? yield* bootstrap() : discoveryFile;
    const text = yield* fs.readFileString(path).pipe(
      Effect.catchIf(
        (error) =>
          error.reason._tag === "NotFound" && configuration.discovery === null && allowStartup,
        () => bootstrap().pipe(Effect.flatMap((next) => fs.readFileString(next))),
      ),
      Effect.catchTag("PlatformError", (cause) =>
        Effect.fail(
          new IntegrationError({ operation: "daemon discovery", reason: "discovery", cause }),
        ),
      ),
    );
    return yield* Schema.decodeUnknownEffect(Schema.fromJsonString(Discovery))(text).pipe(
      Effect.mapError(
        (cause) =>
          new IntegrationError({
            operation: "daemon discovery document",
            reason: "discovery",
            cause,
          }),
      ),
    );
  });

  const call = <A>(
    target: Discovery,
    operation: string,
    token: string,
    request: (
      transport: ReturnType<typeof createConnectTransport>,
      options: CallOptions,
    ) => Promise<A>,
  ) =>
    Effect.tryPromise({
      try: (signal) =>
        request(createConnectTransport({ baseUrl: target.url.origin }), {
          signal,
          timeoutMs: 10_000,
          headers: { Authorization: `Bearer ${token}` },
        }),
      catch: (error) => {
        const cause = ConnectError.from(error);
        const ended = cause.metadata.get("Agx-Launch-Ended");
        return new DaemonError({
          operation,
          code: cause.code,
          ended: Schema.is(EndState)(ended) ? ended : null,
          cause,
        });
      },
    });

  return AgxClient.of({
    discover: discover(),
    poll: discover(false),
    reconnect: (instance) =>
      configuration.discovery === null
        ? bootstrap(instance).pipe(Effect.andThen(discover(false)))
        : discover(false),
    resolve: (target) =>
      call(target, "resolve resources", target.token, (transport, options) =>
        createClient(ResourceService, transport).resolveResources(
          {
            directory: configuration.scopeDirectory,
            harness: configuration.harness,
          },
          options,
        ),
      ),
    prepare: (target, input) =>
      call(target, "prepare resources", target.token, (transport, options) =>
        createClient(ResourceService, transport).prepareResources(input, options),
      ),
    renew: (target, launch, token) =>
      call(target, "renew resources", token, (transport, options) =>
        createClient(LaunchService, transport).renewLaunch({ launchId: launch.id }, options),
      ).pipe(Effect.asVoid),
    attach: (target, input) =>
      call(target, "attach resources", target.token, (transport, options) =>
        createClient(LaunchService, transport).attachLaunch(input, options),
      ).pipe(Effect.asVoid),
    release: (launch, token) =>
      discover(false).pipe(
        Effect.flatMap((target) =>
          call(target, "release resources", token, (transport, options) =>
            createClient(LaunchService, transport).finishLaunch(
              {
                launchId: launch.id,
                outcome: { case: "resourcesReleased", value: true },
              },
              options,
            ),
          ),
        ),
        Effect.asVoid,
        Effect.catchTag("DaemonError", (error) =>
          error.code === Code.NotFound || error.ended !== null ? Effect.void : Effect.fail(error),
        ),
      ),
  });
});

export const AgxClientLive = Layer.effect(AgxClient, makeAgxClient());
