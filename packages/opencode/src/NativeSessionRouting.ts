import { pid } from "node:process";

import * as Service from "@opencode/client/effect/service";
import { fallback } from "@opencode/client/service";
import * as Config from "effect/Config";
import * as Context from "effect/Context";
import * as Effect from "effect/Effect";
import * as FileSystem from "effect/FileSystem";
import * as Layer from "effect/Layer";
import * as PlatformError from "effect/PlatformError";
import * as Schema from "effect/Schema";

export const NativeRoute = Schema.Struct({
  server: Schema.String,
  username: Schema.NullOr(Schema.String),
  password: Schema.NullOr(Schema.String),
});
export type NativeRoute = typeof NativeRoute.Type;

export class NativeRoutingError extends Schema.TaggedError<NativeRoutingError>()(
  "NativeRoutingError",
  {
    reason: Schema.Literals(["custom-server", "discovery", "configuration"]),
    cause: Schema.optionalKey(
      Schema.Union([
        Schema.instanceOf(PlatformError.PlatformError),
        Schema.instanceOf(Schema.SchemaError),
        Schema.instanceOf(Config.ConfigError),
      ]),
    ),
  },
) {
  override get message() {
    return this.reason === "custom-server"
      ? "OpenCode resume requires sessionServer for a server outside the registered shared service"
      : `OpenCode native session routing failed (${this.reason})`;
  }
}

export class NativeSessionRouting extends Context.Service<
  NativeSessionRouting,
  {
    readonly route: (server: URL | null) => Effect.Effect<NativeRoute, NativeRoutingError>;
  }
>()("agx-opencode/NativeSessionRouting") {}

export const makeNativeSessionRouting = Effect.fn("makeNativeSessionRouting")(function* () {
  const fs = yield* FileSystem.FileSystem;
  const route = Effect.fn("NativeSessionRouting.route")(
    function* (server: URL | null) {
      if (server !== null) {
        const password = yield* Config.String("OPENCODE_PASSWORD").pipe(Config.withDefault(""));
        const username = yield* Config.String("OPENCODE_USERNAME").pipe(
          Config.withDefault("opencode"),
        );
        return {
          server: server.href,
          username: password === "" ? null : username,
          password: password === "" ? null : password,
        };
      }
      const file = fallback();
      const text = yield* fs.readFileString(file);
      const registration = yield* Schema.decodeUnknownEffect(Schema.fromJsonString(Service.Info))(
        text,
      );
      // Several native profiles can publish to one daemon. Bind resume to this service process,
      // rather than letting the client's environment silently choose a different profile.
      if (registration.pid !== pid) {
        return yield* new NativeRoutingError({ reason: "custom-server" });
      }
      // This command already runs inside the registered process. Rediscovery could
      // reread a replacement service's route after validating a session in this one.
      return {
        server: registration.url,
        username: registration.password === undefined ? null : "opencode",
        password: registration.password ?? null,
      };
    },
    Effect.catchTags({
      PlatformError: (cause) =>
        Effect.fail(new NativeRoutingError({ reason: "custom-server", cause })),
      SchemaError: (cause) => Effect.fail(new NativeRoutingError({ reason: "discovery", cause })),
      ConfigError: (cause) =>
        Effect.fail(new NativeRoutingError({ reason: "configuration", cause })),
    }),
  );
  return NativeSessionRouting.of({ route });
});

export const NativeSessionRoutingLive = Layer.effect(
  NativeSessionRouting,
  makeNativeSessionRouting(),
);
