import { CpuIcon, RotateCwIcon } from "lucide-react";

import { ConfirmAction } from "~/components/ConfirmAction.tsx";
import { DaemonProblem } from "~/components/DaemonProblem.tsx";
import { Badge } from "~/components/ui/badge.tsx";
import { Button } from "~/components/ui/button.tsx";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "~/components/ui/card.tsx";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "~/components/ui/empty.tsx";
import { Pending } from "~/components/Pending.tsx";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "~/components/ui/table.tsx";
import { toast } from "~/components/ui/toast.tsx";
import { Daemon } from "~/daemon/Daemon.ts";
import { queryKeys, useDaemonMutation, useDaemonQuery } from "~/daemon/query.ts";
import type { McpProcess } from "~/gen/agx/v1/agx_pb.ts";

// Processes have no stream of their own; a short poll keeps idle countdowns and sessions current.
export const refreshInterval = 2000;

export function ProcessesCard() {
  const processes = useDaemonQuery(
    queryKeys.processes,
    Daemon.use((daemon) => daemon.processes),
    {
      refetchInterval: refreshInterval,
    },
  );
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>MCP server processes</CardTitle>
        <CardDescription>Local servers running now, shared or for one session.</CardDescription>
      </CardHeader>
      <CardContent>
        {processes.isPending && <Pending className="h-16" />}
        {processes.isError && <DaemonProblem error={processes.error} />}
        {processes.isSuccess && <ProcessTable processes={processes.data} />}
      </CardContent>
    </Card>
  );
}

/** Processes with restart; for one server's, its column is left out. */
export function ProcessTable({
  processes,
  oneServer = false,
}: {
  readonly processes: ReadonlyArray<McpProcess>;
  readonly oneServer?: boolean;
}) {
  const stop = useDaemonMutation(
    (pid: bigint) => Daemon.use((daemon) => daemon.stopProcess(pid)),
    [queryKeys.processes],
  );
  if (processes.length === 0) {
    return (
      <Empty>
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <CpuIcon />
          </EmptyMedia>
          <EmptyTitle>No processes</EmptyTitle>
          <EmptyDescription>
            Local MCP servers start with the first session that uses them.
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    );
  }
  return (
    <Table>
      <TableHeader>
        <TableRow>
          {!oneServer && <TableHead>Server</TableHead>}
          <TableHead>PID</TableHead>
          <TableHead>Share</TableHead>
          <TableHead>Sessions</TableHead>
          <TableHead>State</TableHead>
          <TableHead>Directory</TableHead>
          <TableHead />
        </TableRow>
      </TableHeader>
      <TableBody>
        {processes.map((process) => (
          <TableRow key={process.pid.toString()}>
            {!oneServer && <TableCell>{process.mcpId}</TableCell>}
            <TableCell className="font-mono">{process.pid.toString()}</TableCell>
            <TableCell className="text-muted-foreground">{process.share}</TableCell>
            <TableCell>
              {process.sessions}{" "}
              {process.launchIds.length > 0 && `· ${process.launchIds.length} launches`}
            </TableCell>
            <TableCell>
              {process.stopsAt === undefined ? (
                <Badge variant="success">in use</Badge>
              ) : (
                <Badge variant="secondary">idle, stops {stopsIn(process.stopsAt.seconds)}</Badge>
              )}
            </TableCell>
            <TableCell className="max-w-md truncate font-mono text-muted-foreground">
              {process.directory}
            </TableCell>
            <TableCell className="text-right">
              <ConfirmAction
                trigger={
                  <Button variant="ghost" size="icon-sm" aria-label="Restart process">
                    <RotateCwIcon />
                  </Button>
                }
                title={`Restart ${process.mcpId}?`}
                description="The process stops and its sessions end; harnesses reconnect and the next session starts it again."
                action="Restart"
                onConfirm={() =>
                  stop.mutate(process.pid, {
                    onError: (error) =>
                      toast.add({
                        type: "error",
                        title: "Restart failed",
                        description: error.message,
                      }),
                  })
                }
              />
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

const relative = new Intl.RelativeTimeFormat(undefined, { numeric: "always", style: "narrow" });

function stopsIn(seconds: bigint): string {
  const remaining = Math.max(0, Number(seconds) - Date.now() / 1000);
  return remaining < 60
    ? relative.format(Math.round(remaining), "second")
    : relative.format(Math.round(remaining / 60), "minute");
}
