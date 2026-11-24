import type { MessageInitShape } from "@bufbuild/protobuf";
import { ConnectError, createClient, type CallOptions, type Transport } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import * as Cause from "effect/Cause";
import * as Context from "effect/Context";
import * as Effect from "effect/Effect";
import * as Layer from "effect/Layer";
import * as Schema from "effect/Schema";
import * as Stream from "effect/Stream";

import {
  SessionProviderService,
  type PublishSessionsRequestSchema,
  type ConnectSessionSourceRequestSchema,
  type ConnectSessionSourceResponse,
  type SessionSourceSchema,
  type ProviderSessionSchema,
} from "../gen/agx/v1/agx_pb.ts";
import { DaemonError } from "./error.ts";
import type { Discovery } from "./schema.ts";

export type PublishInput = MessageInitShape<typeof PublishSessionsRequestSchema>;
export type ConnectInput = MessageInitShape<typeof ConnectSessionSourceRequestSchema>;
export type SessionSourceInput = MessageInitShape<typeof SessionSourceSchema>;
export type ProviderSessionInput = MessageInitShape<typeof ProviderSessionSchema>;

export class SessionProviderClient extends Context.Service<
  SessionProviderClient,
  {
    readonly publish: (target: Discovery, input: PublishInput) => Effect.Effect<void, DaemonError>;
    readonly connect: (
      target: Discovery,
      input: ConnectInput,
    ) => Stream.Stream<ConnectSessionSourceResponse, DaemonError>;
  }
>()("agx-client/SessionProviderClient") {}

const failure = (operation: string, cause: ConnectError) => {
  return new DaemonError({ operation, code: cause.code, ended: null, cause });
};

const call = <A>(
  target: Discovery,
  operation: string,
  request: (transport: Transport, options: CallOptions) => Promise<A>,
) =>
  Effect.tryPromise((signal) =>
    request(createConnectTransport({ baseUrl: target.url.origin }), {
      signal,
      timeoutMs: 10_000,
      headers: { Authorization: `Bearer ${target.token}` },
    }),
  ).pipe(
    Effect.catchTag("UnknownError", (error) =>
      Schema.is(Schema.instanceOf(ConnectError))(error.cause)
        ? Effect.fail(failure(operation, error.cause))
        : Effect.die(error.cause),
    ),
  );

export const makeSessionProviderClient = Effect.fn("makeSessionProviderClient")(() =>
  Effect.sync(() =>
    SessionProviderClient.of({
      publish: Effect.fn("SessionProviderClient.publish")(
        (target: Discovery, input: PublishInput) =>
          call(target, "publish sessions", (transport, options) =>
            createClient(SessionProviderService, transport).publishSessions(input, options),
          ),
        Effect.asVoid,
      ),
      connect: (target, input) =>
        Stream.unwrap(
          Effect.gen(function* () {
            const controller = yield* Effect.acquireRelease(
              Effect.sync(() => new AbortController()),
              (active) => Effect.sync(() => active.abort()),
            );
            const client = createClient(
              SessionProviderService,
              createConnectTransport({ baseUrl: target.url.origin }),
            );
            return Stream.fromAsyncIterable(
              client.connectSessionSource(input, {
                signal: controller.signal,
                headers: { Authorization: `Bearer ${target.token}` },
              }),
              (cause) => new Cause.UnknownError(cause),
            ).pipe(
              Stream.catchTag("UnknownError", (error) =>
                Schema.is(Schema.instanceOf(ConnectError))(error.cause)
                  ? Stream.fail(failure("connect session source", error.cause))
                  : Stream.die(error.cause),
              ),
            );
          }),
        ),
    }),
  ),
);

export const SessionProviderClientLive = Layer.effect(
  SessionProviderClient,
  makeSessionProviderClient(),
);
