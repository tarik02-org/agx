import {
  Code,
  ConnectError,
  createClient,
  type CallOptions,
  type Transport,
} from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import type { MessageInitShape } from "@bufbuild/protobuf";
import * as Context from "effect/Context";
import * as Effect from "effect/Effect";
import * as Layer from "effect/Layer";
import * as Schema from "effect/Schema";

import {
  LaunchService,
  ResourceService,
  type AttachLaunchRequestSchema,
  type PrepareResourcesRequestSchema,
  type ResolveResourcesRequestSchema,
  type PrepareResourcesResponse,
  type ResolveResourcesResponse,
} from "../../../gen/typescript/agx/v1/agx_pb.ts";
import { DaemonError, EndState } from "./error.ts";
import type { Discovery, LifecycleToken } from "./schema.ts";

export type ResolveInput = MessageInitShape<typeof ResolveResourcesRequestSchema>;
export type PrepareInput = MessageInitShape<typeof PrepareResourcesRequestSchema>;
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

export const makeAgxClient = Effect.fn("makeAgxClient")(() =>
  Effect.sync(() => {
    const call = <A>(
      target: Discovery,
      operation: string,
      token: string,
      request: (transport: Transport, options: CallOptions) => Promise<A>,
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

    return AgxClient.of({ resolve, prepare, renew, attach, release });
  }),
);

export const AgxClientLive = Layer.effect(AgxClient, makeAgxClient());
