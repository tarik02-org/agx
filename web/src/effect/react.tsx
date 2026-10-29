import type * as Effect from "effect/Effect";
import type * as Fiber from "effect/Fiber";
import * as Schema from "effect/Schema";
import {
  createContext,
  type DependencyList,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useRef,
} from "react";

import type { Daemon } from "~/daemon/Daemon.ts";
import type { AgxRuntime } from "~/effect/runtime.ts";

/** The services effects run by these hooks may use. */
export type Services = Daemon;

const RuntimeContext = createContext<AgxRuntime | null>(null);

/** A hook below needed the runtime, but no RuntimeProvider is above the component. */
export class RuntimeProviderMissingError extends Schema.TaggedError<RuntimeProviderMissingError>()(
  "RuntimeProviderMissingError",
  {},
) {
  override readonly message = "useRuntime must be used inside RuntimeProvider";
}

export function RuntimeProvider({
  runtime,
  children,
}: {
  readonly runtime: AgxRuntime;
  readonly children: ReactNode;
}) {
  return <RuntimeContext.Provider value={runtime}>{children}</RuntimeContext.Provider>;
}

export function useRuntime(): AgxRuntime {
  const runtime = useContext(RuntimeContext);
  if (runtime === null) {
    throw new RuntimeProviderMissingError();
  }
  return runtime;
}

/**
 * Runs an effect while the component is mounted, restarting it when `deps` change, like
 * `useEffect`. Return `undefined` to run nothing. The fiber is interrupted synchronously on
 * cleanup, so its synchronous finalizers have run before the next effect starts. Failures are
 * the effect's to handle: it may not fail.
 */
export function useFork(
  effect: () => Effect.Effect<unknown, never, Services> | undefined,
  deps: DependencyList,
): void {
  const runtime = useRuntime();
  useEffect(() => {
    const program = effect();
    if (program === undefined) {
      return;
    }
    const fiber = runtime.runFork(program);
    return () => fiber.interruptUnsafe();
    // The caller's deps describe `effect`, as with useEffect.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [runtime, ...deps]);
}

/**
 * Returns an event handler that forks the effect `callback` builds. Fibers still running when
 * the component unmounts are interrupted.
 */
export function useEffectCallback<Args extends ReadonlyArray<unknown>>(
  callback: (...args: Args) => Effect.Effect<unknown, never, Services>,
  deps: DependencyList,
): (...args: Args) => void {
  const runtime = useRuntime();
  const fibers = useRef(new Set<Fiber.Fiber<unknown>>());

  useEffect(() => {
    const running = fibers.current;
    return () => {
      for (const fiber of running) {
        fiber.interruptUnsafe();
      }
      running.clear();
    };
  }, []);

  return useCallback(
    (...args: Args) => {
      const fiber = runtime.runFork(callback(...args));
      if (fiber.pollUnsafe() !== undefined) {
        return;
      }
      fibers.current.add(fiber);
      fiber.addObserver(() => fibers.current.delete(fiber));
    },
    // The caller's deps describe `callback`, as with useCallback.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
    [runtime, ...deps],
  );
}

/**
 * Returns a function that runs an effect for promise-based callers such as TanStack Query.
 * Aborting the signal interrupts it; its failure rejects the promise with the typed error.
 */
export function useRunEffect() {
  const runtime = useRuntime();
  return useCallback(
    <A, E>(
      effect: Effect.Effect<A, E, Services>,
      options?: { readonly signal?: AbortSignal },
    ): Promise<A> => runtime.runPromise(effect, options),
    [runtime],
  );
}
