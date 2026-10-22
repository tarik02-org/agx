import "~/styles.css";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createRouter, RouterProvider } from "@tanstack/react-router";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { RuntimeProvider } from "~/effect/react.tsx";
import { makeAgxRuntime } from "~/effect/runtime.ts";
import { routeTree } from "~/routeTree.gen.ts";

const router = createRouter({ routeTree, basepath: "/ui" });
// Lives as long as the page; nothing to dispose.
const runtime = makeAgxRuntime();
// The daemon is local; failed reads show at once instead of retrying behind a spinner.
const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });

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
