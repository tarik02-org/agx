import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import * as Context from "effect/Context";
import * as Effect from "effect/Effect";
import * as Layer from "effect/Layer";
import * as Predicate from "effect/Predicate";
import * as Schedule from "effect/Schedule";
import * as Stream from "effect/Stream";

import { type DaemonError, daemonError } from "~/daemon/error.ts";
import {
  ConfigurationService,
  type GetConfigurationResponse,
  LaunchService,
  type LoginResponse,
  type McpProcess,
  PackageService,
  type ResolveResponse,
  RuntimeService,
  type RuntimeView,
  SecretService,
  type SecretStatus,
  type WatchOperationResponse,
} from "~/gen/agx/v1/agx_pb.ts";

// The daemon that serves the page, reached with the session cookie its sign-in set.
export class Daemon extends Context.Service<
  Daemon,
  {
    /** The runtime view, again whenever it changes; reconnects after the daemon restarts. */
    readonly runtime: Stream.Stream<RuntimeView, DaemonError>;
    readonly processes: Effect.Effect<ReadonlyArray<McpProcess>, DaemonError>;
    /** Stops an MCP server process; its sessions reconnect and start it again. */
    readonly stopProcess: (pid: bigint) => Effect.Effect<void, DaemonError>;
    readonly revokeLaunch: (launchId: string) => Effect.Effect<void, DaemonError>;
    readonly secrets: Effect.Effect<ReadonlyArray<SecretStatus>, DaemonError>;
    /** An OAuth login: the authorization URL to open, then its outcome. */
    readonly login: (secretId: string) => Stream.Stream<LoginResponse, DaemonError>;
    readonly logout: (secretId: string) => Effect.Effect<void, DaemonError>;
    readonly configuration: Effect.Effect<GetConfigurationResponse, DaemonError>;
    /** What a directory gets, and why; with a harness, its rules and settings too. */
    readonly resolve: (
      directory: string,
      harness: string,
    ) => Effect.Effect<ResolveResponse, DaemonError>;
    readonly completeDirectory: (
      prefix: string,
    ) => Effect.Effect<ReadonlyArray<string>, DaemonError>;
    /** Installs the configuration's packages, reporting progress until the result. */
    readonly install: Stream.Stream<WatchOperationResponse, DaemonError>;
  }
>()("agx-web/daemon/Daemon") {}

// Retry while the daemon restarts; a signed-out page stays signed out.
const reconnect = Schedule.spaced("1 second").pipe(
  Schedule.setInputType<DaemonError>(),
  Schedule.while(({ input }) => input.reason !== "unauthenticated"),
);

// call runs one unary RPC; interrupting the effect aborts the request.
const call = <A>(
  operation: string,
  request: (signal: AbortSignal) => Promise<A>,
): Effect.Effect<A, DaemonError> =>
  Effect.tryPromise({ try: request, catch: (cause) => daemonError(operation, cause) });

// serverStream follows a server-streaming RPC; ending the stream aborts the request.
const serverStream = <A>(
  operation: string,
  open: (signal: AbortSignal) => AsyncIterable<A>,
): Stream.Stream<A, DaemonError> =>
  Stream.unwrap(
    Effect.gen(function* () {
      const controller = yield* Effect.acquireRelease(
        Effect.sync(() => new AbortController()),
        (controller) => Effect.sync(() => controller.abort()),
      );
      return Stream.fromAsyncIterable(open(controller.signal), (cause) =>
        daemonError(operation, cause),
      );
    }),
  );

export const makeDaemon = (): Daemon["Service"] => {
  const transport = createConnectTransport({ baseUrl: globalThis.location.origin });
  const runtime = createClient(RuntimeService, transport);
  const launches = createClient(LaunchService, transport);
  const secrets = createClient(SecretService, transport);
  const packages = createClient(PackageService, transport);
  const configuration = createClient(ConfigurationService, transport);

  return Daemon.of({
    runtime: serverStream("watchRuntime", (signal) => runtime.watchRuntime({}, { signal })).pipe(
      Stream.map((response) => response.runtime),
      Stream.filter(Predicate.isNotUndefined),
      Stream.retry(reconnect),
    ),
    processes: call("listMcpProcesses", (signal) => runtime.listMcpProcesses({}, { signal })).pipe(
      Effect.map((response) => response.processes),
    ),
    stopProcess: (pid) =>
      Effect.asVoid(
        call("stopMcpProcess", (signal) => runtime.stopMcpProcess({ pid }, { signal })),
      ),
    revokeLaunch: (launchId) =>
      Effect.asVoid(
        call("revokeLaunch", (signal) => launches.revokeLaunch({ launchId }, { signal })),
      ),
    secrets: call("getSecretStatus", (signal) => secrets.getSecretStatus({}, { signal })).pipe(
      Effect.map((response) => response.statuses),
    ),
    login: (secretId) => serverStream("login", (signal) => secrets.login({ secretId }, { signal })),
    logout: (secretId) =>
      Effect.asVoid(call("logout", (signal) => secrets.logout({ secretId }, { signal }))),
    configuration: call("getConfiguration", (signal) =>
      configuration.getConfiguration({}, { signal }),
    ),
    resolve: (directory, harness) =>
      call("resolve", (signal) => configuration.resolve({ directory, harness }, { signal })),
    completeDirectory: (prefix) =>
      call("completeDirectory", (signal) =>
        configuration.completeDirectory({ prefix }, { signal }),
      ).pipe(Effect.map((response) => response.directories)),
    install: Stream.unwrap(
      call("install", (signal) =>
        packages.install({ requestId: crypto.randomUUID() }, { signal }),
      ).pipe(
        Effect.map(({ operationId }) =>
          serverStream("watchOperation", (signal) =>
            packages.watchOperation({ operationId }, { signal }),
          ),
        ),
      ),
    ),
  });
};

export const DaemonLive = Layer.sync(Daemon, makeDaemon);
