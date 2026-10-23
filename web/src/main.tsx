import "~/styles.css";

import { MutationCache, QueryCache, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createRouter, RouterProvider } from "@tanstack/react-router";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { useConnection } from "~/daemon/connection.ts";
import { RuntimeProvider } from "~/effect/react.tsx";
import { makeAgxRuntime } from "~/effect/runtime.ts";
import { routeTree } from "~/routeTree.gen.ts";

const router = createRouter({ routeTree, basepath: "/ui", scrollRestoration: true });
// Lives as long as the page; nothing to dispose.
const runtime = makeAgxRuntime();
// The daemon is local; failed reads show at once instead of retrying behind a spinner.
// Every outcome also tells whether the daemon still answers this page at all.
const connection = useConnection.getState();
const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: false } },
  queryCache: new QueryCache({ onSuccess: connection.answered, onError: connection.failed }),
  mutationCache: new MutationCache({ onSuccess: connection.answered, onError: connection.failed }),
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}

const root = document.getElementById("root");
if (root === null) {
  throw new Error("index.html has no #root element");
}
createRoot(root).render(
  <StrictMode>
    <RuntimeProvider runtime={runtime}>
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </RuntimeProvider>
  </StrictMode>,
);
