import { createRootRoute, Outlet } from "@tanstack/react-router";

import { AppShell } from "~/components/AppShell.tsx";

export const Route = createRootRoute({
  component: () => (
    <AppShell>
      <Outlet />
    </AppShell>
  ),
});
