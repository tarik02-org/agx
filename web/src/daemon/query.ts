import { type QueryKey, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type * as Effect from "effect/Effect";

import type { DaemonError } from "~/daemon/error.ts";
import { type Services, useRunEffect } from "~/effect/react.tsx";

/** Cached daemon reads, by what they hold; mutations invalidate them by these keys. */
export const queryKeys = {
  processes: ["mcp-processes"],
  secrets: ["secrets"],
} as const satisfies Record<string, QueryKey>;

/**
 * A cached daemon read. The effect's failure becomes the query's error with its type, since
 * useRunEffect rejects with exactly that value.
 */
export function useDaemonQuery<A>(
  key: QueryKey,
  effect: Effect.Effect<A, DaemonError, Services>,
  options: { readonly refetchInterval?: number } = {},
) {
  const run = useRunEffect();
  return useQuery<A, DaemonError>({
    queryKey: key,
    queryFn: ({ signal }) => run(effect, { signal }),
    ...options,
  });
}

/** A daemon change that refreshes the reads it affects once it finishes. */
export function useDaemonMutation<Args>(
  effect: (args: Args) => Effect.Effect<unknown, DaemonError, Services>,
  invalidates: ReadonlyArray<QueryKey>,
) {
  const run = useRunEffect();
  const client = useQueryClient();
  return useMutation<unknown, DaemonError, Args>({
    mutationFn: (args) => run(effect(args)),
    onSettled: () =>
      Promise.all(invalidates.map((queryKey) => client.invalidateQueries({ queryKey }))),
  });
}
