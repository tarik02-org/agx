import * as Effect from "effect/Effect";
import * as Stream from "effect/Stream";
import { useState } from "react";

import { Daemon } from "~/daemon/Daemon.ts";
import type { DaemonError } from "~/daemon/error.ts";
import { useFork } from "~/effect/react.tsx";
import type { RuntimeView } from "~/gen/agx/v1/agx_pb.ts";

/** The daemon's runtime view as the stream last reported it. */
export type RuntimeViewState =
  | { readonly _tag: "Connecting" }
  | { readonly _tag: "Live"; readonly view: RuntimeView }
  | { readonly _tag: "Failed"; readonly error: DaemonError };

/** Follows the daemon's runtime view while the component is mounted. */
export function useRuntimeView(): RuntimeViewState {
  const [state, setState] = useState<RuntimeViewState>({ _tag: "Connecting" });
  useFork(
    () =>
      Effect.gen(function* () {
        const daemon = yield* Daemon;
        yield* daemon.runtime.pipe(
          Stream.runForEach((view) => Effect.sync(() => setState({ _tag: "Live", view }))),
        );
      }).pipe(
        Effect.catchTag("DaemonError", (error) =>
          Effect.sync(() => setState({ _tag: "Failed", error })),
        ),
      ),
    [],
  );
  return state;
}
