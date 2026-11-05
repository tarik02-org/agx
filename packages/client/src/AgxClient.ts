import type { MessageInitShape } from "@bufbuild/protobuf";
import {
  Code,
  ConnectError,
  createClient,
  type CallOptions,
  type Transport,
} from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import * as Cause from "effect/Cause";
import * as Context from "effect/Context";
import * as Effect from "effect/Effect";
import * as Layer from "effect/Layer";
import * as Schema from "effect/Schema";
import * as Stream from "effect/Stream";

import {
  LaunchService,
  ResourceService,
  type AttachLaunchRequestSchema,
  type PrepareResourcesRequestSchema,
  type ResolveResourcesRequestSchema,
  type RefreshResourcesRequestSchema,
  type WatchResourcesRequestSchema,
  type WatchResourcesResponse,
  type PrepareResourcesResponse,
  type RefreshResourcesResponse,
  type ResolveResourcesResponse,
} from "../gen/agx/v1/agx_pb.ts";
import { DaemonError, EndState } from "./error.ts";
import type { Discovery, LifecycleToken } from "./schema.ts";

export type ResolveInput = MessageInitShape<typeof ResolveResourcesRequestSchema>;
export type PrepareInput = MessageInitShape<typeof PrepareResourcesRequestSchema>;
export type RefreshInput = MessageInitShape<typeof RefreshResourcesRequestSchema>;
export type WatchInput = MessageInitShape<typeof WatchResourcesRequestSchema>;
export type AttachInput = MessageInitShape<typeof AttachLaunchRequestSchema>;

export class AgxClient extends Context.Service<
  AgxClient,
  {
    readonly resolve: (
      target: Discovery,
      input: ResolveInput,
    ) => Effect.Effect<ResolveResourcesResponse, DaemonError>;
    readonly prepare: (
      target: Discovery,
      input: PrepareInput,
    ) => Effect.Effect<PrepareResourcesResponse, DaemonError>;
    readonly refresh: (
      target: Discovery,
      input: RefreshInput,
      token: LifecycleToken,
    ) => Effect.Effect<RefreshResourcesResponse, DaemonError>;
    readonly retire: (
      target: Discovery,
      launchId: string,
      successorLaunchId: string,
      token: LifecycleToken,
    ) => Effect.Effect<void, DaemonError>;
    readonly watch: (
      target: Discovery,
      input: WatchInput,
    ) => Stream.Stream<WatchResourcesResponse, DaemonError>;
    readonly renew: (
      target: Discovery,
      launchId: string,
      token: LifecycleToken,
    ) => Effect.Effect<void, DaemonError>;
    readonly attach: (target: Discovery, input: AttachInput) => Effect.Effect<void, DaemonError>;
    readonly release: (
      target: Discovery,
      launchId: string,
      token: LifecycleToken,
    ) => Effect.Effect<void, DaemonError>;
  }
>()("agx-client/AgxClient") {}

const call = <A>(
  target: Discovery,
  operation: string,
  token: string,
  request: (transport: Transport, options: CallOptions) => Promise<A>,
) =>
  Effect.tryPromise((signal) =>
    request(createConnectTransport({ baseUrl: target.url.origin }), {
      signal,
      timeoutMs: 10_000,
      headers: { Authorization: `Bearer ${token}` },
    }),
  ).pipe(
    Effect.catchTag("UnknownError", (error) => {
      if (!Schema.is(Schema.instanceOf(ConnectError))(error.cause)) {
        return Effect.die(error.cause);
      }
      const cause = error.cause;
      const ended = cause.metadata.get("Agx-Launch-Ended");
      return Effect.fail(
        new DaemonError({
          operation,
          code: cause.code,
          ended: Schema.is(EndState)(ended) ? ended : null,
          cause,
        }),
      );
    }),
  );

const watch = (target: Discovery, input: WatchInput) =>
  Stream.unwrap(
    Effect.fn("AgxClient.watch")(function* () {
      const controller = new AbortController();
      yield* Effect.addFinalizer(() => Effect.sync(() => controller.abort()));
      const responses = createClient(
        ResourceService,
        createConnectTransport({ baseUrl: target.url.origin }),
      ).watchResources(input, {
        signal: controller.signal,
        headers: { Authorization: `Bearer ${target.token}` },
      });
      return Stream.fromAsyncIterable(responses, (cause) => new Cause.UnknownError(cause)).pipe(
        Stream.catchTag("UnknownError", (error) => {
          if (!Schema.is(Schema.instanceOf(ConnectError))(error.cause)) {
            return Stream.die(error.cause);
          }
          const cause = error.cause;
          const ended = cause.metadata.get("Agx-Launch-Ended");
          return Stream.fail(
            new DaemonError({
              operation: "watch resources",
              code: cause.code,
              ended: Schema.is(EndState)(ended) ? ended : null,
              cause,
            }),
          );
        }),
      );
    })(),
  );

export const makeAgxClient = Effect.fn("makeAgxClient")(() =>
  Effect.sync(() => {
    const resolve = Effect.fn("AgxClient.resolve")((target: Discovery, input: ResolveInput) =>
      call(target, "resolve resources", target.token, (transport, options) =>
        createClient(ResourceService, transport).resolveResources(input, options),
      ),
    );

    const prepare = Effect.fn("AgxClient.prepare")((target: Discovery, input: PrepareInput) =>
      call(target, "prepare resources", target.token, (transport, options) =>
        createClient(ResourceService, transport).prepareResources(input, options),
      ),
    );

    const refresh = Effect.fn("AgxClient.refresh")(
      (target: Discovery, input: RefreshInput, token: LifecycleToken) =>
        call(target, "refresh resources", token, (transport, options) =>
          createClient(ResourceService, transport).refreshResources(input, options),
        ),
    );

    const retire = Effect.fn("AgxClient.retire")(
      (target: Discovery, launchId: string, successorLaunchId: string, token: LifecycleToken) =>
        call(target, "retire resources", token, (transport, options) =>
          createClient(ResourceService, transport).retireResources(
            { launchId, successorLaunchId },
            options,
          ),
        ),
      Effect.asVoid,
      Effect.catchTag("DaemonError", (error) =>
        error.code === Code.NotFound || error.ended !== null ? Effect.void : Effect.fail(error),
      ),
    );

    const renew = Effect.fn("AgxClient.renew")(
      (target: Discovery, launchId: string, token: LifecycleToken) =>
        call(target, "renew resources", token, (transport, options) =>
          createClient(LaunchService, transport).renewLaunch({ launchId }, options),
        ),
      Effect.asVoid,
    );

    const attach = Effect.fn("AgxClient.attach")(
      (target: Discovery, input: AttachInput) =>
        call(target, "attach resources", target.token, (transport, options) =>
          createClient(LaunchService, transport).attachLaunch(input, options),
        ),
      Effect.asVoid,
    );

    const release = Effect.fn("AgxClient.release")(
      (target: Discovery, launchId: string, token: LifecycleToken) =>
        call(target, "release resources", token, (transport, options) =>
          createClient(LaunchService, transport).finishLaunch(
            {
              launchId,
              outcome: { case: "resourcesReleased", value: true },
            },
            options,
          ),
        ),
      Effect.asVoid,
      Effect.catchTag("DaemonError", (error) =>
        error.code === Code.NotFound || error.ended !== null ? Effect.void : Effect.fail(error),
      ),
    );

    return AgxClient.of({ resolve, prepare, refresh, retire, watch, renew, attach, release });
  }),
);

export const AgxClientLive = Layer.effect(AgxClient, makeAgxClient());
