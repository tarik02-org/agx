import { useAtomValue } from "@effect/atom-react";
import { createFileRoute } from "@tanstack/react-router";
import { AsyncResult } from "effect/reactivity";

import { DaemonProblem } from "~/components/DaemonProblem.tsx";
import { Badge, type BadgeProps } from "~/components/ui/badge.tsx";
import { Card, CardHeader, CardTitle } from "~/components/ui/card.tsx";
import { Table, TableCell, TableHead, TableRow } from "~/components/ui/table.tsx";
import { DaemonError } from "~/daemon/error.ts";
import { runtimeViewAtom } from "~/daemon/runtime.ts";
import { Launch_State, McpRuntime_State, type RuntimeView } from "~/gen/agx/v1/agx_pb.ts";

export const Route = createFileRoute("/")({ component: RuntimePage });

function RuntimePage() {
  const result = useAtomValue(runtimeViewAtom);
  return AsyncResult.matchWithError(result, {
    onInitial: () => <p className="p-4 text-muted-foreground">Connecting to the daemon…</p>,
    onError: (error) =>
      error instanceof DaemonError ? (
        <DaemonProblem error={error} />
      ) : (
        <p className="p-4 text-muted-foreground">The daemon closed its runtime stream.</p>
      ),
    onDefect: (defect) => <pre className="p-4 font-mono text-destructive">{String(defect)}</pre>,
    onSuccess: ({ value }) => <Runtime view={value} />,
  });
}

const launchStates: Record<
  Launch_State,
  { readonly label: string; readonly tone: BadgeProps["tone"] }
> = {
  [Launch_State.UNSPECIFIED]: { label: "unknown", tone: "neutral" },
  [Launch_State.PREPARED]: { label: "prepared", tone: "neutral" },
  [Launch_State.RUNNING]: { label: "running", tone: "success" },
  [Launch_State.FINISHED]: { label: "finished", tone: "neutral" },
  [Launch_State.REVOKED]: { label: "revoked", tone: "destructive" },
  [Launch_State.EXPIRED]: { label: "expired", tone: "warning" },
};

const mcpStates: Record<
  McpRuntime_State,
  { readonly label: string; readonly tone: BadgeProps["tone"] }
> = {
  [McpRuntime_State.UNSPECIFIED]: { label: "unknown", tone: "neutral" },
  [McpRuntime_State.CONNECTING]: { label: "connecting", tone: "neutral" },
  [McpRuntime_State.READY]: { label: "ready", tone: "success" },
  [McpRuntime_State.NEEDS_AUTH]: { label: "needs login", tone: "warning" },
  [McpRuntime_State.FAILED]: { label: "failed", tone: "destructive" },
};

function Runtime({ view }: { readonly view: RuntimeView }) {
  const active = view.launches.filter(
    (launch) => launch.state === Launch_State.PREPARED || launch.state === Launch_State.RUNNING,
  );
  return (
    <div className="flex flex-col gap-3 p-4">
      <header className="flex items-baseline gap-3">
        <h1 className="font-semibold text-base">Runtime</h1>
        <span className="font-mono text-muted-foreground text-xs">
          daemon {view.version} · {view.daemonInstanceId.slice(0, 12)}
        </span>
      </header>
      <Card>
        <CardHeader>
          <CardTitle>Launches</CardTitle>
          <span className="text-muted-foreground text-xs">{active.length} active</span>
        </CardHeader>
        {view.launches.length === 0 ? (
          <p className="px-3 py-2 text-muted-foreground">No launches.</p>
        ) : (
          <Table>
            <thead>
              <tr>
                <TableHead>Launch</TableHead>
                <TableHead>Harness</TableHead>
                <TableHead>Mode</TableHead>
                <TableHead>State</TableHead>
                <TableHead>Directory</TableHead>
              </tr>
            </thead>
            <tbody>
              {view.launches.map((launch) => (
                <TableRow key={launch.id}>
                  <TableCell className="font-mono text-xs">{launch.id.slice(0, 12)}</TableCell>
                  <TableCell>{launch.harness}</TableCell>
                  <TableCell className="text-muted-foreground">
                    {launch.parentLaunchId === "" ? launch.mode : "thread"}
                  </TableCell>
                  <TableCell>
                    <Badge tone={launchStates[launch.state].tone}>
                      {launchStates[launch.state].label}
                    </Badge>
                  </TableCell>
                  <TableCell className="truncate font-mono text-muted-foreground text-xs">
                    {launch.directory}
                  </TableCell>
                </TableRow>
              ))}
            </tbody>
          </Table>
        )}
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>MCP servers in launches</CardTitle>
        </CardHeader>
        {view.mcpServers.length === 0 ? (
          <p className="px-3 py-2 text-muted-foreground">No MCP servers.</p>
        ) : (
          <Table>
            <thead>
              <tr>
                <TableHead>Server</TableHead>
                <TableHead>Launch</TableHead>
                <TableHead>State</TableHead>
                <TableHead>Problem</TableHead>
              </tr>
            </thead>
            <tbody>
              {view.mcpServers.map((server) => (
                <TableRow key={`${server.launchId}/${server.mcpId}`}>
                  <TableCell>{server.mcpId}</TableCell>
                  <TableCell className="font-mono text-xs">
                    {server.launchId.slice(0, 12)}
                  </TableCell>
                  <TableCell>
                    <Badge tone={mcpStates[server.state].tone}>
                      {mcpStates[server.state].label}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {server.diagnostics.map((d) => d.summary).join("; ")}
                  </TableCell>
                </TableRow>
              ))}
            </tbody>
          </Table>
        )}
      </Card>
    </div>
  );
}
