# AGX Effect client

`agx-client` provides typed Effect operations for resource resolution, preparation
and lease lifecycle calls. It has no OpenCode dependency.

`AgxClientLive` requires no filesystem, process-spawning or connection service.
Each RPC takes an explicit daemon target, and resolution takes a directory and
harness per request. One client can serve multiple consumers and directories.

`AgxConnectionLive` is the optional local adapter. It uses Effect's `FileSystem`
and `ChildProcessSpawner` services to bootstrap `agx`, validate discovery and
recover crashed daemon instances. The application supplies platform layers.

```ts
import * as NodeServices from "@effect/platform-node/NodeServices";
import * as Effect from "effect/Effect";
import * as Layer from "effect/Layer";
import { AgxClient, AgxClientLive } from "agx-client/AgxClient";
import { AgxConnection, AgxConnectionLive } from "agx-client/AgxConnection";

const services = Layer.merge(
  AgxClientLive,
  AgxConnectionLive({ config: "/absolute/path/to/agx.yaml" }),
).pipe(Layer.provide(NodeServices.layer));

const program = Effect.gen(function* () {
  const connection = yield* AgxConnection;
  const client = yield* AgxClient;
  const target = yield* connection.discover();

  return yield* client.resolve(target, {
    directory: "/workspace/project",
    harness: "",
  });
});

await Effect.runPromise(program.pipe(Effect.provide(services)));
```

For an existing connection, decode its discovery document with `Discovery` from
`agx-client/schema` and provide only `AgxClientLive`. The RPC layer does not read
the discovery file itself.

## Modules

| Import                     | Surface                                                               |
| -------------------------- | --------------------------------------------------------------------- |
| `agx-client/AgxClient`     | `AgxClient`, `makeAgxClient`, `AgxClientLive` and request input types |
| `agx-client/AgxConnection` | `AgxConnection`, its factory and configurable local layer             |
| `agx-client/schema`        | Discovery, local connection options and distinct credential schemas   |
| `agx-client/error`         | Connection/RPC errors and the lease termination states                |
| `agx-client/protocol`      | Generated Protobuf messages, schemas and service descriptors          |

The root entry exports these modules as namespaces. Effect remains a peer and
is never bundled; Node platform implementations belong to the application.

Every client and connection operation uses a named `Effect.fn`. Preparation
is not idempotent and is never automatically retried. Callers own lease renewal,
recovery, cleanup and any snapshot-race retry policy. Release accepts a target
explicitly and ignores an already-missing/ended lease; it never starts a daemon.

Local connection `discover()` may bootstrap the daemon; `poll()` only reads
discovery. `reconnect(instance)` requests crash recovery through
`daemon connect --recover-instance`, preserving intentional stops. Supplying an
explicit `discovery` path disables automatic startup/recovery.

Targets retain AGX's authenticated loopback HTTP contract. This package does not
add remote daemon transport. Management tokens, lifecycle tokens and MCP grants
have distinct schema brands. Keep the first two private to the integration.
