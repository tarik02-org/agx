import "~/styles.css";

import { createRouter, RouterProvider } from "@tanstack/react-router";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { RuntimeProvider } from "~/effect/react.tsx";
import { makeAgxRuntime } from "~/effect/runtime.ts";
import { routeTree } from "~/routeTree.gen.ts";

const router = createRouter({ routeTree, basepath: "/ui" });
// Lives as long as the page; nothing to dispose.
const runtime = makeAgxRuntime();

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
      <RouterProvider router={router} />
    </RuntimeProvider>
  </StrictMode>,
);
