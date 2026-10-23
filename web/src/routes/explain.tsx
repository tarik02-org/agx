import { createFileRoute, redirect } from "@tanstack/react-router";

// What a directory gets is the tree's Effective mode, with each item's page saying why. Links
// from before keep working.
export const Route = createFileRoute("/explain")({
  beforeLoad: () => {
    throw redirect({ to: "/" });
  },
});
