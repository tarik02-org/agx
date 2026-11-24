import { create } from "zustand";

import { DaemonError } from "~/daemon/error.ts";

/** Why the page cannot talk to the daemon at all, as opposed to one request failing. */
export type ConnectionLoss = "signed-out" | "unreachable";

interface Connection {
  readonly lost: ConnectionLoss | undefined;
  /** Notes a daemon read's or change's outcome; any answer means the connection works. */
  readonly answered: () => void;
  readonly failed: (error: unknown) => void;
}

export const useConnection = create<Connection>()((set) => ({
  lost: undefined,
  answered: () => set({ lost: undefined }),
  failed: (error) => {
    // Other reasons belong to one request, which shows them where it was made.
    if (error instanceof DaemonError && error.reason === "unauthenticated") {
      set({ lost: "signed-out" });
    } else if (error instanceof DaemonError && error.reason === "unreachable") {
      set({ lost: "unreachable" });
    }
  },
}));
