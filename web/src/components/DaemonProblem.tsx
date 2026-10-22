import { CircleAlertIcon } from "lucide-react";

import { Alert, AlertDescription, AlertTitle } from "~/components/ui/alert.tsx";
import type { DaemonError } from "~/daemon/error.ts";

// Explains a failed daemon call in terms of what the user can do about it.
export function DaemonProblem({ error }: { readonly error: DaemonError }) {
  switch (error.reason) {
    case "unauthenticated":
      return (
        <Problem title="Signed out">
          This page is no longer signed in to the daemon, which happens after it restarts. Run{" "}
          <code className="font-mono">agx ui</code> to sign in again.
        </Problem>
      );
    case "unreachable":
    case "unavailable":
      return (
        <Problem title="Daemon unreachable">
          The daemon does not answer; retrying. Start it with{" "}
          <code className="font-mono">agx daemon start</code>.
        </Problem>
      );
    case "config_invalid":
      return <Problem title="Configuration invalid">{error.message}</Problem>;
    case "invalid_request":
    case "not_found":
    case "conflict":
    case "failed_precondition":
    case "needs_login":
    case "timeout":
    case "internal":
    case "mcp_failed":
      return <Problem title="Request failed">{error.message}</Problem>;
  }
}

function Problem({
  title,
  children,
}: {
  readonly title: string;
  readonly children: React.ReactNode;
}) {
  return (
    <Alert variant="destructive" className="max-w-xl">
      <CircleAlertIcon />
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription>{children}</AlertDescription>
    </Alert>
  );
}
