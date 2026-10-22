import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import * as Context from "effect/Context";
import * as Effect from "effect/Effect";
import * as Layer from "effect/Layer";
import * as Predicate from "effect/Predicate";
import * as Schedule from "effect/Schedule";
import * as Stream from "effect/Stream";

import { RuntimeService, type RuntimeView } from "~/gen/agx/v1/agx_pb.ts";
import { DaemonError, daemonError } from "~/daemon/error.ts";

// The daemon that serves the page, reached with the session cookie its sign-in set.
export class Daemon extends Context.Service<
  Daemon,
  {
    /** The runtime view, again whenever it changes; reconnects after the daemon restarts. */
    readonly runtime: Stream.Stream<RuntimeView, DaemonError>;
  }
>()("agx-web/daemon/Daemon") {}

// Retry while the daemon restarts; a signed-out page stays signed out.
const reconnect = Schedule.spaced("1 second").pipe(
  Schedule.setInputType<DaemonError>(),
  Schedule.while(({ input }) => input.reason !== "unauthenticated"),
);

export const makeDaemon = (): Daemon["Service"] => {
  const transport = createConnectTransport({ baseUrl: globalThis.location.origin });
  const runtime = createClient(RuntimeService, transport);

  const watchRuntime = Stream.unwrap(
    Effect.gen(function* () {
      const controller = yield* Effect.acquireRelease(
        Effect.sync(() => new AbortController()),
        (controller) => Effect.sync(() => controller.abort()),
      );
      return Stream.fromAsyncIterable(
        runtime.watchRuntime({}, { signal: controller.signal }),
        (cause) => daemonError("watchRuntime", cause),
      );
    }),
  ).pipe(
    Stream.map((response) => response.runtime),
    Stream.filter(Predicate.isNotUndefined),
    Stream.retry(reconnect),
  );

  return Daemon.of({ runtime: watchRuntime });
};

export const DaemonLive = Layer.sync(Daemon, makeDaemon);
