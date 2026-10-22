import { useSearch } from "@tanstack/react-router";

import { Daemon } from "~/daemon/Daemon.ts";
import { queryKeys, useDaemonQuery } from "~/daemon/query.ts";
import { resourceKey } from "~/features/explain/Traces.tsx";

/**
 * The directory the UI is scoped to, absolute, or empty for everywhere. It lives in the address
 * (`?dir=`), which every link keeps, so a scoped page can be shared and reloaded.
 */
export const useScopeDirectory = (): string =>
  useSearch({ strict: false, select: (search) => search.dir ?? "" });

/** Expands ~/ against the daemon's home, as scope rules write directories. */
export const absoluteDirectory = (directory: string, home: string): string =>
  directory.startsWith("~/") && home !== "" ? `${home}${directory.slice(1)}` : directory;

/**
 * The resources a launch in the scope directory gets, as `kind/id` keys with the configuration's
 * kind names; undefined while loading or when the UI is not scoped.
 */
export const useEnabledHere = (): ReadonlySet<string> | undefined => {
  const directory = useScopeDirectory();
  const resolved = useDaemonQuery(
    queryKeys.resolve(directory, ""),
    Daemon.use((daemon) => daemon.resolve(directory, "")),
    { enabled: directory !== "" },
  );
  if (directory === "" || !resolved.isSuccess) {
    return undefined;
  }
  return new Set(
    (resolved.data.snapshot?.resources ?? []).map((ref) => {
      const { kind, id } = resourceKey(ref);
      return `${kind === "skill" ? "skills" : kind}/${id}`;
    }),
  );
};
