import { useAtomValue } from "@effect/atom-react";
import { createFileRoute } from "@tanstack/react-router";
import * as Option from "effect/Option";
import { AsyncResult } from "effect/reactivity";
import { PlugIcon, TerminalIcon } from "lucide-react";
import type { ReactNode } from "react";

import { DaemonProblem } from "~/components/DaemonProblem.tsx";
import { Badge } from "~/components/ui/badge.tsx";
import { Card, CardAction, CardContent, CardHeader, CardTitle } from "~/components/ui/card.tsx";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "~/components/ui/empty.tsx";
import { Skeleton } from "~/components/ui/skeleton.tsx";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "~/components/ui/table.tsx";
import { DaemonError } from "~/daemon/error.ts";
import { runtimeViewAtom } from "~/daemon/runtime.ts";
import { Launch_State, McpRuntime_State, type RuntimeView } from "~/gen/agx/v1/agx_pb.ts";

export const Route = createFileRoute("/")({ component: RuntimePage });

function RuntimePage() {
  const result = useAtomValue(runtimeViewAtom);
  return (
    <Page title="Runtime">
      <RuntimeResult result={result} />
    </Page>
  );
}

function RuntimeResult({
  result,
}: {
  readonly result: AsyncResult.AsyncResult<RuntimeView, unknown>;
}) {
  if (AsyncResult.isSuccess(result)) {
    return <Runtime view={result.value} />;
  }
  if (AsyncResult.isInitial(result)) {
    return <Skeleton className="h-40" />;
  }
  const error = AsyncResult.error(result);
  if (Option.isSome(error) && error.value instanceof DaemonError) {
    return <DaemonProblem error={error.value} />;
  }
  return <p className="text-muted-foreground">The daemon closed its runtime stream.</p>;
}

function Page({ title, children }: { readonly title: string; readonly children: ReactNode }) {
  return (
    <div className="flex flex-col gap-4 p-4">
      <h1 className="font-heading font-semibold text-sm">{title}</h1>
      {children}
    </div>
  );
}

type BadgeVariant = "secondary" | "success" | "warning" | "destructive";

const launchStates: Record<
  Launch_State,
  { readonly label: string; readonly variant: BadgeVariant }
> = {
  [Launch_State.UNSPECIFIED]: { label: "unknown", variant: "secondary" },
  [Launch_State.PREPARED]: { label: "prepared", variant: "secondary" },
  [Launch_State.RUNNING]: { label: "running", variant: "success" },
  [Launch_State.FINISHED]: { label: "finished", variant: "secondary" },
  [Launch_State.REVOKED]: { label: "revoked", variant: "destructive" },
  [Launch_State.EXPIRED]: { label: "expired", variant: "warning" },
};

const mcpStates: Record<
  McpRuntime_State,
  { readonly label: string; readonly variant: BadgeVariant }
> = {
  [McpRuntime_State.UNSPECIFIED]: { label: "unknown", variant: "secondary" },
  [McpRuntime_State.CONNECTING]: { label: "connecting", variant: "secondary" },
  [McpRuntime_State.READY]: { label: "ready", variant: "success" },
  [McpRuntime_State.NEEDS_AUTH]: { label: "needs login", variant: "warning" },
  [McpRuntime_State.FAILED]: { label: "failed", variant: "destructive" },
};

function Runtime({ view }: { readonly view: RuntimeView }) {
  const active = view.launches.filter(
    (launch) => launch.state === Launch_State.PREPARED || launch.state === Launch_State.RUNNING,
  );
  return (
    <>
      <p className="font-mono text-muted-foreground text-xs">
        daemon {view.version} · {view.daemonInstanceId.slice(0, 12)}
      </p>
      <Card size="sm">
        <CardHeader>
          <CardTitle>Launches</CardTitle>
          <CardAction>
            <Badge variant="secondary">{active.length} active</Badge>
          </CardAction>
        </CardHeader>
        <CardContent>
          {view.launches.length === 0 ? (
            <Empty>
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <TerminalIcon />
                </EmptyMedia>
                <EmptyTitle>No launches</EmptyTitle>
                <EmptyDescription>Harnesses started through agx show up here.</EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Launch</TableHead>
                  <TableHead>Harness</TableHead>
                  <TableHead>Mode</TableHead>
                  <TableHead>State</TableHead>
                  <TableHead>Directory</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {view.launches.map((launch) => (
                  <TableRow key={launch.id}>
                    <TableCell className="font-mono">{launch.id.slice(0, 12)}</TableCell>
                    <TableCell>{launch.harness}</TableCell>
                    <TableCell className="text-muted-foreground">
                      {launch.parentLaunchId === "" ? launch.mode : "thread"}
                    </TableCell>
                    <TableCell>
                      <Badge variant={launchStates[launch.state].variant}>
                        {launchStates[launch.state].label}
                      </Badge>
                    </TableCell>
                    <TableCell className="max-w-md truncate font-mono text-muted-foreground">
                      {launch.directory}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
      <Card size="sm">
        <CardHeader>
          <CardTitle>MCP servers in launches</CardTitle>
        </CardHeader>
        <CardContent>
          {view.mcpServers.length === 0 ? (
            <Empty>
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <PlugIcon />
                </EmptyMedia>
                <EmptyTitle>No MCP servers</EmptyTitle>
                <EmptyDescription>Servers of running launches show up here.</EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Server</TableHead>
                  <TableHead>Launch</TableHead>
                  <TableHead>State</TableHead>
                  <TableHead>Problem</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {view.mcpServers.map((server) => (
                  <TableRow key={`${server.launchId}/${server.mcpId}`}>
                    <TableCell>{server.mcpId}</TableCell>
                    <TableCell className="font-mono">{server.launchId.slice(0, 12)}</TableCell>
                    <TableCell>
                      <Badge variant={mcpStates[server.state].variant}>
                        {mcpStates[server.state].label}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {server.diagnostics.map((d) => d.summary).join("; ")}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </>
  );
}
