import * as Effect from "effect/Effect";
import { Atom } from "effect/reactivity";
import * as Stream from "effect/Stream";

import { Daemon, DaemonLive } from "~/daemon/Daemon.ts";

export const daemonRuntime = Atom.runtime(DaemonLive);

/** The daemon's runtime view, live while a component reads it. */
export const runtimeViewAtom = daemonRuntime.atom(
  Stream.unwrap(
    Effect.gen(function* () {
      const daemon = yield* Daemon;
      return daemon.runtime;
    }),
  ),
);
