import * as ManagedRuntime from "effect/ManagedRuntime";

import { type Daemon, DaemonLive } from "~/daemon/Daemon.ts";

export type AgxRuntime = ManagedRuntime.ManagedRuntime<Daemon, never>;

/** The page's services; one runtime lives as long as the page. */
export const makeAgxRuntime = (): AgxRuntime => ManagedRuntime.make(DaemonLive);
