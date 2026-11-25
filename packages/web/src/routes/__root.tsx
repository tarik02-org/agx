import { createRootRoute, Outlet, retainSearchParams } from "@tanstack/react-router";
import * as Schema from "effect/Schema";

import { AppShell } from "~/components/AppShell.tsx";
import { ConnectionGate } from "~/components/ConnectionGate.tsx";

// Every page shares the scope: a directory and, for what a launch there gets, a harness. Links
// keep both.
const RootSearch = Schema.Struct({
  dir: Schema.optional(Schema.String),
  harness: Schema.optional(Schema.String),
});

export const Route = createRootRoute({
  validateSearch: Schema.decodeUnknownSync(RootSearch),
  search: { middlewares: [retainSearchParams<typeof RootSearch.Type>(["dir", "harness"])] },
  component: () => (
    <ConnectionGate>
      <AppShell>
        <Outlet />
      </AppShell>
    </ConnectionGate>
  ),
});
