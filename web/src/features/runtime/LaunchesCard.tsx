import { XIcon } from "lucide-react";

import { ConfirmAction } from "~/components/ConfirmAction.tsx";
import { Badge } from "~/components/ui/badge.tsx";
import { Button } from "~/components/ui/button.tsx";
import { Card, CardAction, CardContent, CardHeader, CardTitle } from "~/components/ui/card.tsx";
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
import { queryKeys, useDaemonMutation } from "~/daemon/query.ts";
import { isActiveLaunch, launchStates } from "~/features/runtime/states.ts";
import type { Launch } from "~/gen/agx/v1/agx_pb.ts";

export function LaunchesCard({ launches }: { readonly launches: ReadonlyArray<Launch> }) {
  const revoke = useDaemonMutation(
    (launchId: string) => Daemon.use((daemon) => daemon.revokeLaunch(launchId)),
    [queryKeys.processes],
  );
  const active = launches.filter((launch) => isActiveLaunch(launch.state));
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>Launches</CardTitle>
        <CardAction>
          <Badge variant="secondary">{active.length} active</Badge>
        </CardAction>
      </CardHeader>
      <CardContent>
        {launches.length === 0 ? (
          <p className="text-muted-foreground text-xs">
            No launches. Harnesses started through agx show up here.
          </p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Launch</TableHead>
                <TableHead>Harness</TableHead>
                <TableHead>Mode</TableHead>
                <TableHead>State</TableHead>
                <TableHead>Directory</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {launches.map((launch) => (
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
                  <TableCell className="text-right">
                    {isActiveLaunch(launch.state) && (
                      <ConfirmAction
                        trigger={
                          <Button variant="ghost" size="icon-sm" aria-label="Revoke launch">
                            <XIcon />
                          </Button>
                        }
                        title="Revoke this launch?"
                        description={`${launch.harness} in ${launch.directory} loses its MCP servers at once. The harness keeps running.`}
                        action="Revoke"
                        onConfirm={() =>
                          revoke.mutate(launch.id, {
                            onError: (error) =>
                              toast.add({
                                type: "error",
                                title: "Revoking failed",
                                description: error.message,
                              }),
                          })
                        }
                      />
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  );
}
