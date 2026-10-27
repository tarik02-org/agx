import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import type { MessageInitShape } from "@bufbuild/protobuf";
import * as Context from "effect/Context";
import * as Effect from "effect/Effect";
import * as Layer from "effect/Layer";
import * as Predicate from "effect/Predicate";
import * as Schedule from "effect/Schedule";
import * as Schema from "effect/Schema";
import * as Stream from "effect/Stream";

import { DaemonError, daemonError } from "~/daemon/error.ts";
import { sessionHeaders } from "~/daemon/session.ts";
import { ConfigSchema } from "~/lib/configSchema.ts";
import {
  type ConfigChangeSchema,
  ConfigurationService,
  type Definition,
  type GetConfigurationResponse,
  type GetSkillResponse,
  type InstallResponse,
  type PlannedFile,
  type PreviewSourceResponse,
  type SourceRef,
  type CompleteDirectoryResponse,
  type InspectSourceResponse,
  InspectSourceRequestSchema,
  type ReadFileResponse,
  type ReadPreviewFileResponse,
  LaunchService,
  type LoginResponse,
  type CallToolResponse,
  type McpProcess,
  type McpTools,
  PackageService,
  type ResolveResponse,
  RuntimeService,
  type RuntimeView,
  SecretService,
  type SecretStatus,
} from "~/gen/agx/v1/agx_pb.ts";

// The daemon that serves the page, reached with the session its sign-in handed over.
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
    /** Stores a keychain secret's value. */
    readonly setSecret: (secretId: string, value: string) => Effect.Effect<void, DaemonError>;
    /**
     * A configured MCP server with its tools, starting it if needed; directory is where a stdio
     * server without cwd runs. A server that cannot start fails.
     */
    readonly tools: (directory: string, mcpId: string) => Effect.Effect<McpTools, DaemonError>;
    readonly callTool: (
      directory: string,
      mcpId: string,
      tool: string,
      argumentsJson: string,
    ) => Effect.Effect<CallToolResponse, DaemonError>;
    readonly configuration: Effect.Effect<GetConfigurationResponse, DaemonError>;
    /** What a directory gets, and why; with a harness, its rules and settings too. */
    readonly resolve: (
      directory: string,
      harness: string,
    ) => Effect.Effect<ResolveResponse, DaemonError>;
    readonly completeDirectory: (
      prefix: string,
    ) => Effect.Effect<ReadonlyArray<string>, DaemonError>;
    /** Directories and files completing a path, for picking a file. */
    readonly completePath: (
      prefix: string,
    ) => Effect.Effect<CompleteDirectoryResponse, DaemonError>;
    /** Installs the configuration's packages, reporting progress until the result. */
    readonly install: Stream.Stream<InstallResponse, DaemonError>;
    /** The configuration JSON Schema of the daemon's version and harnesses. */
    readonly configSchema: Effect.Effect<ConfigSchema, DaemonError>;
    readonly definitions: Effect.Effect<ReadonlyArray<Definition>, DaemonError>;
    readonly skill: (skillId: string) => Effect.Effect<GetSkillResponse, DaemonError>;
    /** A configuration fragment, or a file of a configured skill or instructions definition. */
    readonly readFile: (path: string) => Effect.Effect<ReadFileResponse, DaemonError>;
    /**
     * The diff each file would get, with the hashes the apply checks against. New definitions,
     * rules, and packages go to file, or the only writable fragment when it is empty.
     */
    readonly planChanges: (
      changes: ReadonlyArray<ConfigChangeInit>,
      file: string,
    ) => Effect.Effect<ReadonlyArray<PlannedFile>, DaemonError>;
    /** Writes every change at once, or none when a planned file changed meanwhile. */
    readonly applyChanges: (
      changes: ReadonlyArray<ConfigChangeInit>,
      file: string,
      expected: ReadonlyArray<PlannedFile>,
    ) => Effect.Effect<ReadonlyArray<string>, DaemonError>;
    /** Fetches a Git source and lists the skills it would add. */
    readonly previewSource: (source: string) => Effect.Effect<PreviewSourceResponse, DaemonError>;
    /** A Git source's branches and tags, without fetching it. */
    readonly sourceRefs: (git: string) => Effect.Effect<ReadonlyArray<SourceRef>, DaemonError>;
    /** A Git source checked out at its pins: its commit, content hash, and directories. */
    readonly inspectSource: (
      request: MessageInitShape<typeof InspectSourceRequestSchema>,
    ) => Effect.Effect<InspectSourceResponse, DaemonError>;
    readonly readPreviewFile: (
      previewId: string,
      skill: string,
      path: string,
    ) => Effect.Effect<ReadPreviewFileResponse, DaemonError>;
  }
>()("agx-web/daemon/Daemon") {}

/** One staged configuration change, as the editor builds it. */
export type ConfigChangeInit = MessageInitShape<typeof ConfigChangeSchema>;

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

const schemaFailure = (reason: DaemonError["reason"], message: string) =>
  new DaemonError({ operation: "configSchema", reason, message, files: [] });

// fetchSchema reads the schema the daemon serves beside the UI, outside its RPCs.
const fetchSchema = (origin: string): Effect.Effect<ConfigSchema, DaemonError> =>
  Effect.gen(function* () {
    const response = yield* Effect.tryPromise({
      try: (signal) => fetch(`${origin}/ui/schema.json`, { signal, headers: sessionHeaders() }),
      catch: () => schemaFailure("unreachable", "no daemon answered"),
    });
    if (response.status === 401) {
      return yield* schemaFailure("unauthenticated", "the UI session ended");
    }
    if (!response.ok) {
      return yield* schemaFailure("internal", `the daemon answered ${response.status}`);
    }
    const body = yield* Effect.tryPromise({
      try: () => response.text(),
      catch: () => schemaFailure("unreachable", "the response broke off"),
    });
    return yield* Schema.decodeUnknownEffect(Schema.fromJsonString(ConfigSchema))(body).pipe(
      Effect.mapError(() => schemaFailure("internal", "the schema is not a JSON Schema object")),
    );
  });

export const makeDaemon = (): Daemon["Service"] => {
  const transport = createConnectTransport({
    baseUrl: globalThis.location.origin,
    interceptors: [
      (next) => (request) => {
        for (const [name, value] of Object.entries(sessionHeaders())) {
          request.header.set(name, value);
        }
        return next(request);
      },
    ],
  });
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
    setSecret: (secretId, value) =>
      Effect.asVoid(
        call("setSecret", (signal) =>
          secrets.setSecret({ secretId, value: new TextEncoder().encode(value) }, { signal }),
        ),
      ),
    tools: (directory, mcpId) =>
      call("listTools", (signal) =>
        runtime.listTools({ scope: { directory, all: true }, mcpId }, { signal }),
      ).pipe(
        Effect.flatMap((response) =>
          response.servers[0] === undefined
            ? Effect.die(new Error(`listTools returned no server for ${mcpId}`))
            : Effect.succeed(response.servers[0]),
        ),
      ),
    callTool: (directory, mcpId, tool, argumentsJson) =>
      call("callTool", (signal) =>
        runtime.callTool(
          { scope: { directory, all: true }, mcpId, tool, argumentsJson },
          { signal },
        ),
      ),
    configuration: call("getConfiguration", (signal) =>
      configuration.getConfiguration({}, { signal }),
    ),
    resolve: (directory, harness) =>
      call("resolve", (signal) => configuration.resolve({ directory, harness }, { signal })),
    completeDirectory: (prefix) =>
      call("completeDirectory", (signal) =>
        configuration.completeDirectory({ prefix }, { signal }),
      ).pipe(Effect.map((response) => response.directories)),
    completePath: (prefix) =>
      call("completePath", (signal) =>
        configuration.completeDirectory({ prefix, files: true }, { signal }),
      ),
    install: serverStream("install", (signal) => packages.install({}, { signal })),
    configSchema: fetchSchema(globalThis.location.origin),
    definitions: call("listDefinitions", (signal) =>
      configuration.listDefinitions({}, { signal }),
    ).pipe(Effect.map((response) => response.definitions)),
    skill: (skillId) =>
      call("getSkill", (signal) => configuration.getSkill({ skillId }, { signal })),
    readFile: (path) => call("readFile", (signal) => configuration.readFile({ path }, { signal })),
    planChanges: (changes, file) =>
      call("planChanges", (signal) =>
        configuration.planChanges({ changes: [...changes], file }, { signal }),
      ).pipe(Effect.map((response) => response.files)),
    applyChanges: (changes, file, expected) =>
      call("applyChanges", (signal) =>
        configuration.applyChanges(
          { changes: [...changes], file, expected: [...expected] },
          { signal },
        ),
      ).pipe(Effect.map((response) => response.written)),
    previewSource: (source) =>
      call("previewSource", (signal) => packages.previewSource({ source }, { signal })),
    inspectSource: (request) =>
      call("inspectSource", (signal) => packages.inspectSource(request, { signal })),
    sourceRefs: (git) =>
      call("listSourceRefs", (signal) => packages.listSourceRefs({ git }, { signal })).pipe(
        Effect.map((response) => response.refs),
      ),
    readPreviewFile: (previewId, skill, path) =>
      call("readPreviewFile", (signal) =>
        packages.readPreviewFile({ previewId, skill, path }, { signal }),
      ),
  });
};

export const DaemonLive = Layer.sync(Daemon, makeDaemon);
