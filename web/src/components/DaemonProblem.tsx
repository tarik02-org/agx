import type { DaemonError } from "~/daemon/error.ts";

// Explains a failed daemon call in terms of what the user can do about it.
export function DaemonProblem({ error }: { readonly error: DaemonError }) {
  switch (error.reason) {
    case "unauthenticated":
      return (
        <Notice title="Signed out">
          This page is no longer signed in to the daemon, which happens after it restarts. Run{" "}
          <code className="font-mono">agx ui</code> to sign in again.
        </Notice>
      );
    case "unreachable":
    case "unavailable":
      return (
        <Notice title="Daemon unreachable">
          The daemon does not answer; retrying. Start it with{" "}
          <code className="font-mono">agx daemon start</code>.
        </Notice>
      );
    case "config_invalid":
      return <Notice title="Configuration invalid">{error.message}</Notice>;
    case "invalid_request":
    case "not_found":
    case "conflict":
    case "failed_precondition":
    case "needs_login":
    case "timeout":
    case "internal":
    case "mcp_failed":
      return <Notice title="Request failed">{error.message}</Notice>;
  }
}

function Notice({
  title,
  children,
}: {
  readonly title: string;
  readonly children: React.ReactNode;
}) {
  return (
    <div className="m-4 max-w-xl rounded-lg border border-destructive/30 bg-destructive/5 p-3">
      <div className="font-medium text-destructive">{title}</div>
      <p className="mt-1 text-muted-foreground">{children}</p>
    </div>
  );
}
