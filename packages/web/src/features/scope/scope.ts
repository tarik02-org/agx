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

/** The harness the scope looks at, or empty for what every harness gets. */
export const useScopeHarness = (): string =>
  useSearch({ strict: false, select: (search) => search.harness ?? "" });

/** Expands ~/ against the daemon's home, as scope rules write directories. */
export const absoluteDirectory = (directory: string, home: string): string =>
  directory.startsWith("~/") && home !== "" ? `${home}${directory.slice(1)}` : directory;

/**
 * What a launch in the scope directory gets for the scope's harness, with why: the daemon's
 * resolution, or nothing while the UI is not scoped.
 */
export const useResolution = () => {
  const directory = useScopeDirectory();
  const harness = useScopeHarness();
  return useDaemonQuery(
    queryKeys.resolve(directory, harness),
    Daemon.use((daemon) => daemon.resolve(directory, harness)),
    { enabled: directory !== "" },
  );
};

/**
 * The resources a launch in the scope directory gets, as `kind/id` keys with the configuration's
 * kind names; undefined while loading or when the UI is not scoped.
 */
export const useEnabledHere = (): ReadonlySet<string> | undefined => {
  const resolved = useResolution();
  if (!resolved.isSuccess) {
    return undefined;
  }
  return new Set(
    (resolved.data.snapshot?.resources ?? []).map((ref) => {
      const { kind, id } = resourceKey(ref);
      return `${kind === "skill" ? "skills" : kind}/${id}`;
    }),
  );
};
